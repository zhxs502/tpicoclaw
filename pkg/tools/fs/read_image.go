package fstools

import (
	"context"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/media"
	"golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// ReadImageTool loads a local image, downscales/compresses it (pure Go) when
// it exceeds the configured limits, registers the result in the MediaStore and
// returns a media:// reference. The agent loop's resolveMediaRefs will then
// base64-encode it and attach it as an image part in the next LLM request.
//
// Unlike LoadImageTool, ReadImageTool is meant for vision-capable providers:
// it actively shrinks large images so the uploaded payload stays cheap and
// small (DeepSeek Files API bills per image, and inline base64 grows quickly).
//
// Everything is done with the Go standard library + golang.org/x/image, so no
// external ffmpeg/ffprobe binaries are required (works on Android/Windows).
type ReadImageTool struct {
	workspace    string
	restrict     bool
	maxFileSize  int64 // source admission limit (before compression)
	maxDimension int   // long-edge cap in pixels (0 = default 1280)
	quality      int   // jpeg quality (0 = default 80)
	mediaStore   media.MediaStore
	allowPaths   []*regexp.Regexp

	defaultChannel string
	defaultChatID  string
}

const (
	defaultReadImageMaxDimension = 1280
	defaultReadImageQuality      = 80
	readImageSourceLimitBytes    = 100 * 1024 * 1024 // allow big sources; we compress before storing
)

func NewReadImageTool(
	workspace string,
	restrict bool,
	store media.MediaStore,
	allowPaths ...[]*regexp.Regexp,
) *ReadImageTool {
	var patterns []*regexp.Regexp
	if len(allowPaths) > 0 {
		patterns = allowPaths[0]
	}
	return &ReadImageTool{
		workspace:    workspace,
		restrict:     restrict,
		maxFileSize:  readImageSourceLimitBytes,
		maxDimension: defaultReadImageMaxDimension,
		quality:      defaultReadImageQuality,
		mediaStore:   store,
		allowPaths:   patterns,
	}
}

func (t *ReadImageTool) Name() string { return "read_image" }

func (t *ReadImageTool) Description() string {
	return "Load a local image file so you can analyze its contents with vision. " +
		"Supported formats: JPEG, PNG, GIF, WebP, BMP. " +
		"Large images are automatically downscaled and compressed before being sent to the model. " +
		"After calling this tool, describe or analyze the image in your next response."
}

func (t *ReadImageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the local image file. Relative paths are resolved from workspace.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *ReadImageTool) SetContext(channel, chatID string) {
	t.defaultChannel = channel
	t.defaultChatID = chatID
}

func (t *ReadImageTool) SetMediaStore(store media.MediaStore) {
	t.mediaStore = store
}

func (t *ReadImageTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	path, _ := args["path"].(string)
	if strings.TrimSpace(path) == "" {
		return ErrorResult("path is required")
	}

	channel := ToolChannel(ctx)
	if channel == "" {
		channel = t.defaultChannel
	}
	chatID := ToolChatID(ctx)
	if chatID == "" {
		chatID = t.defaultChatID
	}
	if channel == "" || chatID == "" {
		return ErrorResult("no target channel/chat available")
	}
	if t.mediaStore == nil {
		return ErrorResult("media store not configured")
	}

	resolved, err := validatePathWithAllowPaths(path, t.workspace, t.restrict, t.allowPaths)
	if err != nil {
		return ErrorResult(fmt.Sprintf("invalid path: %v", err))
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return ErrorResult(fmt.Sprintf("file not found: %v", err))
	}
	if info.IsDir() {
		return ErrorResult("path is a directory, expected an image file")
	}
	if info.Size() > t.maxFileSize {
		return ErrorResult(fmt.Sprintf(
			"file too large: %d bytes (max %d bytes)", info.Size(), t.maxFileSize,
		))
	}

	mediaType := detectMediaType(resolved)
	if !strings.HasPrefix(mediaType, "image/") {
		return ErrorResult(fmt.Sprintf(
			"file does not appear to be an image (detected type: %s)", mediaType,
		))
	}

	// Determine source dimensions by decoding the header only (pure Go).
	cfg, err := decodeImageConfig(resolved, mediaType)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return ErrorResult(fmt.Sprintf(
			"could not read image dimensions for %s (%v)", resolved, err,
		))
	}
	srcW, srcH := cfg.Width, cfg.Height

	storePath := resolved
	storedType := mediaType
	storeSize := info.Size()

	// Compress only when the image exceeds the dimension/byte limits.
	needCompress := srcW > t.maxDimension || srcH > t.maxDimension || info.Size() > int64(config.DefaultMaxMediaSize)
	if needCompress {
		outPath, compType, compSize, cerr := t.compressImage(resolved, mediaType)
		if cerr == nil {
			storePath = outPath
			storedType = compType
			storeSize = compSize
		} else {
			return ErrorResult(fmt.Sprintf("failed to compress image: %v", cerr))
		}
	}

	filename := filepath.Base(resolved)
	scope := fmt.Sprintf("tool:read_image:%s:%s", channel, chatID)
	ref, err := t.mediaStore.Store(storePath, media.MediaMeta{
		Filename:      filename,
		ContentType:   storedType,
		Source:        "tool:read_image",
		CleanupPolicy: media.CleanupPolicyForgetOnly,
	}, scope)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to register image in media store: %v", err))
	}

	msg := fmt.Sprintf("Image loaded: %s\n[image: photo]", filename)
	if needCompress {
		outW, outH := scaledDimensions(srcW, srcH, t.maxDimension)
		msg = fmt.Sprintf(
			"Image loaded and compressed: %s\n  original: %dx%d, %s\n  sent as: %dx%d %s, %s\n[image: photo]",
			filename, srcW, srcH, humanBytes(info.Size()),
			outW, outH, storedType, humanBytes(storeSize),
		)
	}

	return &ToolResult{
		ForLLM:  msg,
		ForUser: fmt.Sprintf("Loaded image: %s", filename),
		Media:   []string{ref},
	}
}

// compressImage decodes the source, downscales it to fit maxDimension and
// re-encodes it as JPEG. Uses only Go image libraries (no ffmpeg dependency).
func (t *ReadImageTool) compressImage(src, mediaType string) (string, string, int64, error) {
	img, err := decodeImage(src, mediaType)
	if err != nil {
		return "", "", 0, fmt.Errorf("decode: %v", err)
	}

	out := scaleToFit(img, t.maxDimension)
	dir := media.TempDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", 0, err
	}

	tmp, err := os.CreateTemp(dir, "read-image-*.jpg")
	if err != nil {
		return "", "", 0, err
	}
	defer tmp.Close()

	if err := jpeg.Encode(tmp, out, &jpeg.Options{Quality: t.quality}); err != nil {
		return "", "", 0, fmt.Errorf("encode: %v", err)
	}
	compInfo, err := os.Stat(tmp.Name())
	if err != nil || compInfo.Size() == 0 {
		return "", "", 0, fmt.Errorf("encode produced no output")
	}
	return tmp.Name(), "image/jpeg", compInfo.Size(), nil
}

// scaleToFit downscales src so its long edge is at most maxDim, never enlarging.
func scaleToFit(src image.Image, maxDim int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || maxDim <= 0 {
		return src
	}
	if w <= maxDim && h <= maxDim {
		return src
	}
	outW, outH := scaledDimensions(w, h, maxDim)
	dst := image.NewRGBA(image.Rect(0, 0, outW, outH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

// decodeImageConfig reads only the header to obtain width/height (pure Go).
func decodeImageConfig(path, mediaType string) (image.Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return image.Config{}, err
	}
	defer f.Close()
	switch strings.ToLower(mediaType) {
	case "image/jpeg":
		return jpeg.DecodeConfig(f)
	case "image/png":
		return png.DecodeConfig(f)
	case "image/gif":
		return gif.DecodeConfig(f)
	case "image/webp":
		return webp.DecodeConfig(f)
	case "image/bmp":
		return bmp.DecodeConfig(f)
	}
	cfg, _, err := image.DecodeConfig(f)
	return cfg, err
}

// decodeImage fully decodes the source image (pure Go).
func decodeImage(path, mediaType string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	switch strings.ToLower(mediaType) {
	case "image/jpeg":
		return jpeg.Decode(f)
	case "image/png":
		return png.Decode(f)
	case "image/gif":
		return gif.Decode(f)
	case "image/webp":
		return webp.Decode(f)
	case "image/bmp":
		return bmp.Decode(f)
	}
	img, _, err := image.Decode(f)
	return img, err
}

func scaledDimensions(w, h, maxDim int) (int, int) {
	if w <= 0 || h <= 0 || maxDim <= 0 {
		return w, h
	}
	if w <= maxDim && h <= maxDim {
		return w, h
	}
	scale := float64(maxDim) / float64(maxInt(w, h))
	return int(float64(w) * scale), int(float64(h) * scale)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func humanBytes(n int64) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
