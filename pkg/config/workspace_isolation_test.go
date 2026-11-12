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

func TestAPIKeyFingerprint(t *testing.T) {
	if got := APIKeyFingerprint(""); got != "" {
		t.Fatalf("APIKeyFingerprint(\"\") = %q, want empty", got)
	}
	if got := APIKeyFingerprint("   "); got != "" {
		t.Fatalf("APIKeyFingerprint(blank) = %q, want empty", got)
	}

	first := APIKeyFingerprint("sk-test-key")
	second := APIKeyFingerprint("  sk-test-key  ")
	if first == "" {
		t.Fatal("fingerprint should not be empty")
	}
	if first != second {
		t.Fatalf("fingerprint should ignore surrounding whitespace: %q != %q", first, second)
	}
	if !IsAPIKeyFingerprint(first) {
		t.Fatalf("IsAPIKeyFingerprint(%q) = false, want true", first)
	}
	if APIKeyFingerprint("sk-test-key-2") == first {
		t.Fatal("different keys should produce different fingerprints")
	}
	if IsAPIKeyFingerprint("sha256:xyz") {
		t.Fatal("malformed fingerprint should be rejected")
	}
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

func TestWorkspaceForModelConfig_UsesBoundProfile(t *testing.T) {
	defaultWorkspace := filepath.Join(t.TempDir(), "default")
	isolatedWorkspace := filepath.Join(t.TempDir(), "isolated")
	key := "sk-bound"
	fingerprint := APIKeyFingerprint(key)

	cfg := &Config{
		Agents: AgentsConfig{Defaults: AgentDefaults{Workspace: defaultWorkspace}},
		ModelList: SecureModelList{
			&ModelConfig{ModelName: "bound-model", Model: "openai/gpt-4o", APIKeys: SimpleSecureStrings(key)},
			{ModelName: "free-model", Model: "openai/gpt-4o-mini", APIKeys: SimpleSecureStrings("sk-other")},
		},
		WorkspaceIsolation: &WorkspaceIsolationConfig{
			Enabled: boolValue(true),
			Profiles: []WorkspaceProfileConfig{
				{
					ID:                 "team-a",
					Name:               "Team A",
					Path:               isolatedWorkspace,
					APIKeyFingerprints: []string{fingerprint},
				},
			},
		},
	}

	workspace, profile := cfg.WorkspaceForModelConfig(cfg.ModelList[0])
	if workspace != isolatedWorkspace {
		t.Fatalf("workspace = %q, want %q", workspace, isolatedWorkspace)
	}
	if profile == nil || profile.ID != "team-a" {
		t.Fatalf("profile = %#v, want team-a", profile)
	}

	workspace, profile = cfg.WorkspaceForModelConfig(cfg.ModelList[1])
	if workspace != defaultWorkspace {
		t.Fatalf("unbound workspace = %q, want %q", workspace, defaultWorkspace)
	}
	if profile != nil {
		t.Fatalf("unbound profile = %#v, want nil", profile)
	}

	workspace, _ = cfg.WorkspaceForModelConfig(nil)
	if workspace != defaultWorkspace {
		t.Fatalf("nil model workspace = %q, want %q", workspace, defaultWorkspace)
	}
}

func TestWorkspaceIsolation_DisabledIgnoresProfiles(t *testing.T) {
	defaultWorkspace := filepath.Join(t.TempDir(), "default")
	key := "sk-bound"
	cfg := &Config{
		Agents: AgentsConfig{Defaults: AgentDefaults{Workspace: defaultWorkspace}},
		ModelList: SecureModelList{
			&ModelConfig{ModelName: "m", Model: "openai/gpt-4o", APIKeys: SimpleSecureStrings(key)},
		},
		WorkspaceIsolation: &WorkspaceIsolationConfig{
			Enabled: boolValue(false),
			Profiles: []WorkspaceProfileConfig{
				{ID: "a", Path: filepath.Join(t.TempDir(), "a"), APIKeyFingerprints: []string{APIKeyFingerprint(key)}},
			},
		},
	}

	workspace, profile := cfg.WorkspaceForModelConfig(cfg.ModelList[0])
	if workspace != defaultWorkspace || profile != nil {
		t.Fatalf("disabled isolation returned workspace=%q profile=%#v", workspace, profile)
	}
}

func TestWorkspaceIsolation_ImplicitEnableWithProfiles(t *testing.T) {
	key := "sk-bound"
	cfg := &Config{
		Agents: AgentsConfig{Defaults: AgentDefaults{Workspace: filepath.Join(t.TempDir(), "default")}},
		ModelList: SecureModelList{
			&ModelConfig{ModelName: "m", Model: "openai/gpt-4o", APIKeys: SimpleSecureStrings(key)},
		},
		WorkspaceIsolation: &WorkspaceIsolationConfig{
			Profiles: []WorkspaceProfileConfig{
				{ID: "a", Path: filepath.Join(t.TempDir(), "a"), APIKeyFingerprints: []string{APIKeyFingerprint(key)}},
			},
		},
	}
	if cfg.WorkspaceIsolation == nil || !cfg.WorkspaceIsolation.IsEnabled() {
		t.Fatal("profiles should enable isolation when enabled is nil")
	}
	if _, profile := cfg.WorkspaceForModelConfig(cfg.ModelList[0]); profile == nil {
		t.Fatal("expected a bound profile")
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
	fp1 := APIKeyFingerprint("sk-1")
	fp2 := APIKeyFingerprint("sk-2")
	cfg := &WorkspaceIsolationConfig{
		Profiles: []WorkspaceProfileConfig{
			{ID: "", Name: "Team A", Path: "  " + filepath.Join(t.TempDir(), "team-a") + "  ", APIKeyFingerprints: []string{" " + strings.ToUpper(fp1) + " ", fp1}},
			{ID: "", Name: "Team A", Path: filepath.Join(t.TempDir(), "team-b"), APIKeyFingerprints: []string{fp2}},
			{ID: "team-a", Name: "Other", Path: filepath.Join(t.TempDir(), "team-c")},
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
	if len(cfg.Profiles[0].APIKeyFingerprints) != 1 || cfg.Profiles[0].APIKeyFingerprints[0] != fp1 {
		t.Fatalf("fingerprints = %#v, want [%s]", cfg.Profiles[0].APIKeyFingerprints, fp1)
	}
	if cfg.Profiles[0].Path != strings.TrimSpace(cfg.Profiles[0].Path) {
		t.Fatal("path should be trimmed")
	}
}

func TestWorkspaceIsolation_Validate(t *testing.T) {
	base := filepath.Join(t.TempDir(), "ws")
	fp := APIKeyFingerprint("sk-1")

	valid := &WorkspaceIsolationConfig{
		Enabled: boolValue(true),
		Profiles: []WorkspaceProfileConfig{
			{ID: "a", Path: filepath.Join(base, "a"), APIKeyFingerprints: []string{fp}},
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
		{
			name: "invalid fingerprint",
			profiles: []WorkspaceProfileConfig{
				{ID: "a", Path: filepath.Join(base, "x"), APIKeyFingerprints: []string{"sha256:zzzz"}},
			},
			want: "invalid api key fingerprint",
		},
		{
			name: "duplicate fingerprint",
			profiles: []WorkspaceProfileConfig{
				{ID: "a", Path: filepath.Join(base, "x"), APIKeyFingerprints: []string{fp}},
				{ID: "b", Path: filepath.Join(base, "y"), APIKeyFingerprints: []string{fp}},
			},
			want: "already bound",
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
