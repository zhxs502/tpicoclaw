package openai_compat

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/logger"
)

// DeepSeek Files API support (OpenAI-compatible /files endpoint).
//
// The DeepSeek vision models (e.g. deepseek-v4-flash-vision-exp) consume
// images primarily through the Files API: upload the bytes once, then
// reference the returned file id inside a user message as
// {"type":"file","file_id":"..."}. Inline base64 image_url parts are often
// silently ignored by the provider, so picoclaw now uploads images for
// DeepSeek providers and falls back to inline image_url only when the Files
// API fails.

const (
	deepSeekFileIDPrefix       = "file_id:"
	deepSeekFileExpirySeconds  = 7 * 24 * 60 * 60 // provider min 3600, max 2592000; 7d matches dsh defaults
	deepSeekFileUploadMaxBytes = 128 * 1024 * 1024
)

type deepSeekFileObject struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	Bytes     int64  `json:"bytes"`
	CreatedAt int64  `json:"created_at"`
	Filename  string `json:"filename"`
	Purpose   string `json:"purpose"`
	ExpiresAt *int64 `json:"expires_at"`
}

// isDeepSeekProvider reports whether this provider routes to DeepSeek.
func (p *Provider) isDeepSeekProvider() bool {
	return strings.EqualFold(strings.TrimSpace(p.providerName), "deepseek") || isDeepSeekHost(p.apiBase)
}

// uploadDeepSeekImages replaces inline image data URLs in message media with
// DeepSeek Files API references. Upload failures keep the original data URL so
// the request can still proceed through the inline image_url fallback.
func (p *Provider) uploadDeepSeekImages(messages []Message) []Message {
	if !p.isDeepSeekProvider() || p.apiKey == "" {
		return messages
	}
	out := make([]Message, 0, len(messages))
	for _, m := range messages {
		if len(m.Media) == 0 {
			out = append(out, m)
			continue
		}
		media := make([]string, 0, len(m.Media))
		for _, ref := range m.Media {
			if strings.HasPrefix(ref, deepSeekFileIDPrefix) {
				media = append(media, ref)
				continue
			}
			if strings.HasPrefix(ref, "data:image/") {
				fileID, err := p.uploadImageDataURL(ref)
				if err == nil {
					media = append(media, deepSeekFileIDPrefix+fileID)
					continue
				}
				logger.WarnCF("openai_compat", "DeepSeek Files API upload failed; falling back to inline image_url", map[string]any{
					"error": err.Error(),
				})
			}
			media = append(media, ref)
		}
		m.Media = media
		out = append(out, m)
	}
	return out
}

// uploadImageDataURL decodes a base64 data URL and uploads it to the DeepSeek
// Files API, returning the provider file id.
func (p *Provider) uploadImageDataURL(dataURL string) (string, error) {
	comma := strings.IndexByte(dataURL, ',')
	if comma <= 0 {
		return "", fmt.Errorf("invalid image data URL")
	}
	meta := dataURL[:comma]
	payload := dataURL[comma+1:]
	if !strings.Contains(strings.ToLower(meta), ";base64") {
		return "", fmt.Errorf("image data URL is not base64-encoded")
	}
	mimeType := strings.TrimPrefix(meta, "data:")
	if semi := strings.IndexByte(mimeType, ';'); semi >= 0 {
		mimeType = mimeType[:semi]
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.NewReplacer("\n", "", "\r", "", "\t", "", " ", "").Replace(payload))
	if err != nil {
		return "", fmt.Errorf("decode image data URL: %w", err)
	}
	if len(decoded) > deepSeekFileUploadMaxBytes {
		return "", fmt.Errorf("image exceeds %d MiB upload limit", deepSeekFileUploadMaxBytes/(1024*1024))
	}

	ext := ".bin"
	switch strings.ToLower(mimeType) {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "image/webp":
		ext = ".webp"
	case "image/gif":
		ext = ".gif"
	}
	filename := "picoclaw-image-" + strconv.FormatInt(time.Now().UnixNano(), 36) + ext

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("purpose", "user_data")
	_ = mw.WriteField("expires_after[anchor]", "created_at")
	_ = mw.WriteField("expires_after[seconds]", strconv.FormatInt(deepSeekFileExpirySeconds, 10))
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("create multipart file field: %w", err)
	}
	if _, err := fw.Write(decoded); err != nil {
		return "", fmt.Errorf("write multipart file field: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("close multipart body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(p.apiBase, "/")+"/files", &body)
	if err != nil {
		return "", fmt.Errorf("build files request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	for k, v := range p.customHeaders {
		req.Header.Set(k, v)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("files API request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read files API response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := strings.TrimSpace(string(respBody))
		if len(detail) > 300 {
			detail = detail[:300] + "..."
		}
		return "", fmt.Errorf("files API HTTP %d: %s", resp.StatusCode, detail)
	}

	var fileObj deepSeekFileObject
	if err := json.Unmarshal(respBody, &fileObj); err != nil {
		return "", fmt.Errorf("parse files API response: %w", err)
	}
	if fileObj.ID == "" {
		return "", fmt.Errorf("files API response missing file id")
	}
	if fileObj.Purpose != "user_data" {
		logger.DebugCF("openai_compat", "DeepSeek Files API returned unexpected purpose", map[string]any{
			"purpose": fileObj.Purpose,
		})
	}
	return fileObj.ID, nil
}
