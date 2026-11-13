package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/sipeed/picoclaw/pkg"
)

// WorkspaceIsolationConfig defines named workspace directories that can be
// selected per chat. Profiles are a simple name -> path mapping; the Web UI
// lets the user pick one next to the chat model selector.
type WorkspaceIsolationConfig struct {
	// Enabled switches workspace selection on. When nil, it is enabled as soon
	// as at least one profile is configured.
	Enabled  *bool                    `json:"enabled,omitempty"`
	Profiles []WorkspaceProfileConfig `json:"profiles,omitempty"`
}

// WorkspaceProfileConfig is one named workspace.
type WorkspaceProfileConfig struct {
	// ID is a stable identifier used by the Web UI and the chat selector. It
	// must match [A-Za-z0-9][A-Za-z0-9_-]{0,63}.
	ID string `json:"id"`
	// Name is the human readable label shown in the Web UI.
	Name string `json:"name,omitempty"`
	// Path is the workspace directory. It may start with "~".
	Path string `json:"path"`
	// RestrictToWorkspace is deprecated and ignored. File/exec restrictions
	// always come from agents.defaults; the field is kept only so configs
	// written by older builds still load, and is cleared on save.
	RestrictToWorkspace *bool `json:"restrict_to_workspace,omitempty"`
	// AllowReadOutsideWorkspace is deprecated for the same reason.
	AllowReadOutsideWorkspace *bool `json:"allow_read_outside_workspace,omitempty"`
	// APIKeyFingerprints is deprecated. It is kept only so configs written by
	// the previous api-key binding implementation still load; runtime and the
	// Web UI ignore it, and NormalizeWorkspaceIsolation clears it on save.
	APIKeyFingerprints []string `json:"api_key_fingerprints,omitempty"`
}

const (
	// MaxWorkspaceProfiles caps the number of workspaces in one config.
	MaxWorkspaceProfiles = 64
)

var workspaceProfileIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

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

// IsEnabled reports whether workspace selection should be applied.
func (w *WorkspaceIsolationConfig) IsEnabled() bool {
	if w == nil {
		return false
	}
	if w.Enabled != nil {
		return *w.Enabled
	}
	return len(w.Profiles) > 0
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

// WorkspaceProfileByID returns the profile with the given id. Lookups are
// case-insensitive so IDs remain stable across the Web UI and config edits.
func (c *Config) WorkspaceProfileByID(id string) *WorkspaceProfileConfig {
	if c == nil || c.WorkspaceIsolation == nil {
		return nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	for i := range c.WorkspaceIsolation.Profiles {
		profile := &c.WorkspaceIsolation.Profiles[i]
		if strings.EqualFold(profile.ID, id) {
			return profile
		}
	}
	return nil
}

// WorkspacePathByID returns the expanded path of a workspace profile.
func (c *Config) WorkspacePathByID(id string) (string, *WorkspaceProfileConfig, bool) {
	profile := c.WorkspaceProfileByID(id)
	if profile == nil {
		return "", nil, false
	}
	path := ExpandWorkspacePath(profile.Path)
	if path == "" {
		return "", nil, false
	}
	return path, profile, true
}

// NormalizeWorkspaceIsolation fills in generated IDs and trims fields in place.
// It returns true when anything changed.
func NormalizeWorkspaceIsolation(cfg *WorkspaceIsolationConfig) bool {
	if cfg == nil {
		return false
	}
	changed := false
	seenIDs := make(map[string]struct{}, len(cfg.Profiles))

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
		if len(profile.APIKeyFingerprints) > 0 {
			// Drop bindings from the deprecated api-key based implementation.
			profile.APIKeyFingerprints = nil
			changed = true
		}
		if profile.RestrictToWorkspace != nil || profile.AllowReadOutsideWorkspace != nil {
			// Security settings are global now; clear legacy per-workspace
			// overrides so they cannot silently restrict a workspace again.
			profile.RestrictToWorkspace = nil
			profile.AllowReadOutsideWorkspace = nil
			changed = true
		}

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
	}
	return changed
}

// Validate checks profile identifiers and paths.
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
