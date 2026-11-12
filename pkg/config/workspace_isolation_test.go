package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func boolValue(v bool) *bool {
	return &v
}

func TestExpandWorkspacePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home directory unavailable")
	}
	if got, want := ExpandWorkspacePath("~/ws"), filepath.Join(home, "ws"); got != want {
		t.Fatalf("ExpandWorkspacePath(~/ws) = %q, want %q", got, want)
	}
	if got, want := ExpandWorkspacePath(`~\ws`), filepath.Join(home, "ws"); got != want {
		t.Fatalf(`ExpandWorkspacePath(~\ws) = %q, want %q`, got, want)
	}
	if got := ExpandWorkspacePath("  "); got != "" {
		t.Fatalf("ExpandWorkspacePath(blank) = %q, want empty", got)
	}
}

func TestWorkspaceIsolation_EnabledDefaults(t *testing.T) {
	cfg := &WorkspaceIsolationConfig{}
	if cfg.IsEnabled() {
		t.Fatal("empty isolation config should be disabled")
	}

	cfg.Profiles = []WorkspaceProfileConfig{{ID: "a", Path: filepath.Join(t.TempDir(), "a")}}
	if !cfg.IsEnabled() {
		t.Fatal("profiles should enable isolation when enabled is nil")
	}

	cfg.Enabled = boolValue(false)
	if cfg.IsEnabled() {
		t.Fatal("explicit enabled=false should win")
	}
}

func TestWorkspaceProfileByID(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		WorkspaceIsolation: &WorkspaceIsolationConfig{
			Profiles: []WorkspaceProfileConfig{
				{ID: "team-a", Name: "Team A", Path: filepath.Join(dir, "a")},
				{ID: "team-b", Path: filepath.Join(dir, "b")},
			},
		},
	}
	profile := cfg.WorkspaceProfileByID("TEAM-A")
	if profile == nil || profile.Name != "Team A" {
		t.Fatalf("profile = %#v, want team-a", profile)
	}
	path, profile, ok := cfg.WorkspacePathByID("team-b")
	if !ok || profile == nil || path != filepath.Join(dir, "b") {
		t.Fatalf("path=%q profile=%#v ok=%v", path, profile, ok)
	}
	if _, _, ok := cfg.WorkspacePathByID("missing"); ok {
		t.Fatal("unknown workspace should not resolve")
	}
}

func TestWorkspaceProfile_EffectiveRestrictions(t *testing.T) {
	profile := &WorkspaceProfileConfig{}
	if !profile.EffectiveRestrict(true) {
		t.Fatal("nil override should keep the default restrict value")
	}
	if profile.EffectiveAllowReadOutside(false) {
		t.Fatal("nil override should keep the default allow-read value")
	}

	profile.RestrictToWorkspace = boolValue(false)
	profile.AllowReadOutsideWorkspace = boolValue(true)
	if profile.EffectiveRestrict(true) {
		t.Fatal("explicit override should win")
	}
	if !profile.EffectiveAllowReadOutside(false) {
		t.Fatal("explicit allow-read override should win")
	}
}

func TestNormalizeWorkspaceIsolation(t *testing.T) {
	dir := t.TempDir()
	cfg := &WorkspaceIsolationConfig{
		Profiles: []WorkspaceProfileConfig{
			{ID: "", Name: "Team A", Path: "  " + filepath.Join(dir, "team-a") + "  "},
			{ID: "", Name: "Team A", Path: filepath.Join(dir, "team-b")},
			{ID: "team-a", Name: "Other", Path: filepath.Join(dir, "team-c")},
		},
	}
	if !NormalizeWorkspaceIsolation(cfg) {
		t.Fatal("NormalizeWorkspaceIsolation should report changes")
	}
	if cfg.Profiles[0].ID != "team-a" {
		t.Fatalf("first id = %q, want team-a", cfg.Profiles[0].ID)
	}
	if cfg.Profiles[1].ID != "team-a-2" {
		t.Fatalf("second id = %q, want team-a-2", cfg.Profiles[1].ID)
	}
	if cfg.Profiles[2].ID != "team-a-3" {
		t.Fatalf("third id = %q, want team-a-3", cfg.Profiles[2].ID)
	}
	if cfg.Profiles[0].Path != strings.TrimSpace(cfg.Profiles[0].Path) {
		t.Fatal("path should be trimmed")
	}
}

func TestWorkspaceIsolation_Validate(t *testing.T) {
	base := filepath.Join(t.TempDir(), "ws")

	valid := &WorkspaceIsolationConfig{
		Enabled: boolValue(true),
		Profiles: []WorkspaceProfileConfig{
			{ID: "a", Path: filepath.Join(base, "a")},
			{ID: "b", Path: filepath.Join(base, "b")},
		},
	}
	if errs := valid.Validate(); len(errs) != 0 {
		t.Fatalf("valid config rejected: %v", errs)
	}

	tests := []struct {
		name     string
		profiles []WorkspaceProfileConfig
		want     string
	}{
		{
			name:     "missing id",
			profiles: []WorkspaceProfileConfig{{ID: "", Path: filepath.Join(base, "x")}},
			want:     "id is required",
		},
		{
			name:     "invalid id",
			profiles: []WorkspaceProfileConfig{{ID: "bad id", Path: filepath.Join(base, "x")}},
			want:     "may only contain",
		},
		{
			name: "duplicate id",
			profiles: []WorkspaceProfileConfig{
				{ID: "a", Path: filepath.Join(base, "x")},
				{ID: "A", Path: filepath.Join(base, "y")},
			},
			want: "duplicate id",
		},
		{
			name:     "relative path",
			profiles: []WorkspaceProfileConfig{{ID: "a", Path: "relative/ws"}},
			want:     "must be absolute",
		},
		{
			name: "duplicate path",
			profiles: []WorkspaceProfileConfig{
				{ID: "a", Path: filepath.Join(base, "x")},
				{ID: "b", Path: filepath.Join(base, "x")},
			},
			want: "duplicates workspace",
		},
		{
			name: "nested path",
			profiles: []WorkspaceProfileConfig{
				{ID: "a", Path: filepath.Join(base, "x")},
				{ID: "b", Path: filepath.Join(base, "x", "nested")},
			},
			want: "nested inside",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			errs := (&WorkspaceIsolationConfig{Enabled: boolValue(true), Profiles: tc.profiles}).Validate()
			joined := strings.Join(errs, "\n")
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("errors = %v, want substring %q", errs, tc.want)
			}
		})
	}
}

func TestLoadConfig_AcceptsLegacyAPIKeyFingerprints(t *testing.T) {
	cfg := DefaultConfig()
	cfg.WorkspaceIsolation = &WorkspaceIsolationConfig{
		Enabled: boolValue(true),
		Profiles: []WorkspaceProfileConfig{
			{
				ID:                 "team-a",
				Name:               "Team A",
				Path:               filepath.Join(t.TempDir(), "team-a"),
				APIKeyFingerprints: []string{"sha256:0123456789abcdef"},
			},
		},
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("legacy config should still load: %v", err)
	}
	if loaded.WorkspaceIsolation == nil || len(loaded.WorkspaceIsolation.Profiles) != 1 {
		t.Fatalf("workspace isolation = %#v", loaded.WorkspaceIsolation)
	}
	if got := len(loaded.WorkspaceIsolation.Profiles[0].APIKeyFingerprints); got != 1 {
		t.Fatalf("legacy fingerprints = %d, want 1 after load", got)
	}

	normalized := *loaded.WorkspaceIsolation
	NormalizeWorkspaceIsolation(&normalized)
	if got := len(normalized.Profiles[0].APIKeyFingerprints); got != 0 {
		t.Fatalf("fingerprints = %d, want cleared by normalization", got)
	}
}
