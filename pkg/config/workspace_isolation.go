package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/sipeed/picoclaw/pkg"
)

// WorkspaceIsolationConfig binds API keys to isolated workspace directories.
//
// Every configured model entry may carry one or more API keys. When isolation
// is enabled, an agent whose primary model uses a key that belongs to a
// workspace profile runs with that profile's path as its workspace (and,
// optionally, with its own sandbox restriction settings). Keys that are not
// bound to any profile keep using agents.defaults.workspace.
type WorkspaceIsolationConfig struct {
	// Enabled switches isolation on. When nil, isolation is enabled as soon as
	// at least one profile is configured.
	Enabled  *bool                    `json:"enabled,omitempty"`
	Profiles []WorkspaceProfileConfig `json:"profiles,omitempty"`
}

// WorkspaceProfileConfig is one isolated workspace.
type WorkspaceProfileConfig struct {
	// ID is a stable identifier used by the Web UI and the bindings. It must
	// match [A-Za-z0-9][A-Za-z0-9_-]{0,63}.
	ID string `json:"id"`
	// Name is the human readable label shown in the Web UI.
	Name string `json:"name,omitempty"`
	// Path is the workspace directory. It may start with "~".
	Path string `json:"path"`
	// RestrictToWorkspace optionally overrides
	// agents.defaults.restrict_to_workspace for agents bound to this profile.
	RestrictToWorkspace *bool `json:"restrict_to_workspace,omitempty"`
	// AllowReadOutsideWorkspace optionally overrides
	// agents.defaults.allow_read_outside_workspace for bound agents.
	AllowReadOutsideWorkspace *bool `json:"allow_read_outside_workspace,omitempty"`
	// APIKeyFingerprints lists the API keys routed to this workspace. Values
	// are produced by APIKeyFingerprint; the keys themselves are never stored
	// here.
	APIKeyFingerprints []string `json:"api_key_fingerprints,omitempty"`
}

const (
	apiKeyFingerprintPrefix = "sha256:"
	// MaxWorkspaceProfiles caps the number of isolated workspaces in one config.
	MaxWorkspaceProfiles = 64
)

var workspaceProfileIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// APIKeyFingerprint returns a stable, non-reversible identifier for an API key.
//
// The fingerprint is a truncated SHA-256 of the resolved key value, which makes
// it stable across encrypted (enc://) and file (file://) references. An empty
// key produces an empty fingerprint and therefore never matches a profile.
func APIKeyFingerprint(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return apiKeyFingerprintPrefix + hex.EncodeToString(sum[:8])
}

// IsAPIKeyFingerprint reports whether value looks like a fingerprint produced
// by APIKeyFingerprint.
func IsAPIKeyFingerprint(value string) bool {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, apiKeyFingerprintPrefix) {
		return false
	}
	digest := strings.TrimPrefix(value, apiKeyFingerprintPrefix)
	if len(digest) != 16 {
		return false
	}
	for _, r := range digest {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

// ExpandWorkspacePath expands a leading "~" and cleans the path. Both "/" and
// "\" are accepted after the home shorthand.
func ExpandWorkspacePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		return home
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		return filepath.Join(home, path[2:])
	}
	return filepath.Clean(path)
}

// IsEnabled reports whether workspace isolation should be applied.
func (w *WorkspaceIsolationConfig) IsEnabled() bool {
	if w == nil {
		return false
	}
	if w.Enabled != nil {
		return *w.Enabled
	}
	return len(w.Profiles) > 0
}

// EffectiveRestrict resolves the restrict_to_workspace value for a bound agent.
func (p *WorkspaceProfileConfig) EffectiveRestrict(defaultValue bool) bool {
	if p == nil || p.RestrictToWorkspace == nil {
		return defaultValue
	}
	return *p.RestrictToWorkspace
}

// EffectiveAllowReadOutside resolves allow_read_outside_workspace for a bound agent.
func (p *WorkspaceProfileConfig) EffectiveAllowReadOutside(defaultValue bool) bool {
	if p == nil || p.AllowReadOutsideWorkspace == nil {
		return defaultValue
	}
	return *p.AllowReadOutsideWorkspace
}

// DefaultWorkspacePath returns the expanded default workspace path.
func (c *Config) DefaultWorkspacePath() string {
	if c == nil {
		return ""
	}
	path := ExpandWorkspacePath(c.Agents.Defaults.Workspace)
	if path == "" {
		path = ExpandWorkspacePath(filepath.Join(GetHome(), pkg.WorkspaceName))
	}
	return path
}

// WorkspaceProfileForAPIKey returns the profile bound to key, or nil.
func (c *Config) WorkspaceProfileForAPIKey(key string) *WorkspaceProfileConfig {
	if c == nil || c.WorkspaceIsolation == nil || !c.WorkspaceIsolation.IsEnabled() {
		return nil
	}
	fingerprint := APIKeyFingerprint(key)
	if fingerprint == "" {
		return nil
	}
	for i := range c.WorkspaceIsolation.Profiles {
		profile := &c.WorkspaceIsolation.Profiles[i]
		for _, bound := range profile.APIKeyFingerprints {
			if strings.EqualFold(strings.TrimSpace(bound), fingerprint) {
				return profile
			}
		}
	}
	return nil
}

// WorkspaceForModelConfig returns the workspace path and matching profile for a
// model entry. The default workspace is returned when the model has no key, the
// key is not bound, or isolation is disabled.
func (c *Config) WorkspaceForModelConfig(mc *ModelConfig) (string, *WorkspaceProfileConfig) {
	if c == nil {
		return "", nil
	}
	defaultPath := c.DefaultWorkspacePath()
	if mc == nil {
		return defaultPath, nil
	}
	profile := c.WorkspaceProfileForAPIKey(mc.APIKey())
	if profile == nil {
		return defaultPath, nil
	}
	path := ExpandWorkspacePath(profile.Path)
	if path == "" {
		return defaultPath, nil
	}
	return path, profile
}

// PrimaryModelConfig returns the first configured entry for a model alias or
// model reference. Unlike GetModelConfig it never round-robins, which keeps
// workspace resolution deterministic for load-balanced aliases.
func (c *Config) PrimaryModelConfig(modelName string) *ModelConfig {
	if c == nil {
		return nil
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil
	}
	for i := range c.ModelList {
		if c.ModelList[i] != nil && c.ModelList[i].ModelName == modelName {
			return c.ModelList[i]
		}
	}
	return nil
}

// ActiveWorkspaceForModel resolves the effective workspace for a model alias.
func (c *Config) ActiveWorkspaceForModel(modelName string) (string, *WorkspaceProfileConfig) {
	if c == nil {
		return "", nil
	}
	return c.WorkspaceForModelConfig(c.PrimaryModelConfig(modelName))
}

// NormalizeWorkspaceIsolation fills in generated IDs, trims fields and removes
// duplicate fingerprints in place. It returns true when anything changed.
func NormalizeWorkspaceIsolation(cfg *WorkspaceIsolationConfig) bool {
	if cfg == nil {
		return false
	}
	changed := false
	seenIDs := make(map[string]struct{}, len(cfg.Profiles))
	usedFingerprints := make(map[string]string, len(cfg.Profiles))

	for i := range cfg.Profiles {
		profile := &cfg.Profiles[i]
		trimmedID := strings.TrimSpace(profile.ID)
		trimmedName := strings.TrimSpace(profile.Name)
		trimmedPath := strings.TrimSpace(profile.Path)
		if profile.ID != trimmedID || profile.Name != trimmedName || profile.Path != trimmedPath {
			changed = true
		}
		profile.ID = trimmedID
		profile.Name = trimmedName
		profile.Path = trimmedPath

		if profile.ID == "" {
			profile.ID = workspaceProfileSlug(profile.Name)
			changed = true
		}
		baseID := profile.ID
		for suffix := 2; ; suffix++ {
			key := strings.ToLower(profile.ID)
			if _, exists := seenIDs[key]; !exists {
				seenIDs[key] = struct{}{}
				break
			}
			profile.ID = fmt.Sprintf("%s-%d", baseID, suffix)
			changed = true
		}

		fingerprints := make([]string, 0, len(profile.APIKeyFingerprints))
		for _, fingerprint := range profile.APIKeyFingerprints {
			fingerprint = strings.ToLower(strings.TrimSpace(fingerprint))
			if fingerprint == "" {
				changed = true
				continue
			}
			if owner, exists := usedFingerprints[fingerprint]; exists && owner != strings.ToLower(profile.ID) {
				// A key may only belong to one workspace; the first profile wins.
				changed = true
				continue
			}
			if containsFoldedString(fingerprints, fingerprint) {
				changed = true
				continue
			}
			usedFingerprints[fingerprint] = strings.ToLower(profile.ID)
			fingerprints = append(fingerprints, fingerprint)
		}
		profile.APIKeyFingerprints = fingerprints
	}
	return changed
}

// Validate checks profile identifiers, paths and fingerprints.
func (w *WorkspaceIsolationConfig) Validate() []string {
	if w == nil {
		return nil
	}
	var errs []string
	if len(w.Profiles) > MaxWorkspaceProfiles {
		errs = append(errs, fmt.Sprintf("too many workspace profiles: %d (max %d)", len(w.Profiles), MaxWorkspaceProfiles))
		return errs
	}

	ids := make(map[string]struct{}, len(w.Profiles))
	paths := make(map[string]string, len(w.Profiles))
	fingerprints := make(map[string]string, len(w.Profiles))

	for i := range w.Profiles {
		profile := &w.Profiles[i]
		label := profile.ID
		if label == "" {
			label = fmt.Sprintf("#%d", i+1)
		}
		if profile.ID == "" {
			errs = append(errs, fmt.Sprintf("workspace %s: id is required", label))
		} else if !workspaceProfileIDPattern.MatchString(profile.ID) {
			errs = append(errs, fmt.Sprintf("workspace %s: id may only contain letters, digits, '-' and '_'", label))
		} else if _, exists := ids[strings.ToLower(profile.ID)]; exists {
			errs = append(errs, fmt.Sprintf("workspace %s: duplicate id", label))
		} else {
			ids[strings.ToLower(profile.ID)] = struct{}{}
		}

		if profile.Path == "" {
			errs = append(errs, fmt.Sprintf("workspace %s: path is required", label))
		} else {
			expanded := ExpandWorkspacePath(profile.Path)
			if expanded == "" {
				errs = append(errs, fmt.Sprintf("workspace %s: path could not be expanded", label))
			} else if !filepath.IsAbs(expanded) {
				errs = append(errs, fmt.Sprintf("workspace %s: path must be absolute or start with '~': %s", label, profile.Path))
			} else {
				key := workspacePathKey(expanded)
				if other, exists := paths[key]; exists {
					errs = append(errs, fmt.Sprintf("workspace %s: path duplicates workspace %s", label, other))
				} else {
					paths[key] = label
				}
			}
		}

		for _, fingerprint := range profile.APIKeyFingerprints {
			fingerprint = strings.ToLower(strings.TrimSpace(fingerprint))
			if fingerprint == "" {
				continue
			}
			if !IsAPIKeyFingerprint(fingerprint) {
				errs = append(errs, fmt.Sprintf("workspace %s: invalid api key fingerprint %q", label, fingerprint))
				continue
			}
			if owner, exists := fingerprints[fingerprint]; exists && owner != label {
				errs = append(errs, fmt.Sprintf("workspace %s: api key is already bound to workspace %s", label, owner))
				continue
			}
			fingerprints[fingerprint] = label
		}
	}

	errs = append(errs, validateWorkspacePathNesting(paths)...)
	return errs
}

func validateWorkspacePathNesting(paths map[string]string) []string {
	var errs []string
	for pathA, labelA := range paths {
		for pathB, labelB := range paths {
			if pathA == pathB {
				continue
			}
			if isWorkspaceSubpath(pathA, pathB) {
				errs = append(errs, fmt.Sprintf(
					"workspace %s: path %s is nested inside workspace %s; isolated workspaces must not overlap",
					labelB, pathB, labelA,
				))
			}
		}
	}
	return errs
}

func isWorkspaceSubpath(parent, child string) bool {
	parent = workspacePathKey(parent)
	child = workspacePathKey(child)
	if parent == "" || child == "" || parent == child {
		return false
	}
	sep := string(os.PathSeparator)
	if !strings.HasSuffix(parent, sep) {
		parent += sep
	}
	return strings.HasPrefix(child, parent)
}

func workspacePathKey(path string) string {
	clean := filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(clean)
	}
	return clean
}

func workspaceProfileSlug(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "workspace"
	}
	if len(slug) > 64 {
		slug = slug[:64]
	}
	if slug[0] < 'a' || slug[0] > 'z' {
		if slug[0] < '0' || slug[0] > '9' {
			slug = "workspace-" + slug
		}
	}
	return slug
}

func containsFoldedString(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}
