package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func newWorkspacesTestServer(t *testing.T) (string, *http.ServeMux, func()) {
	t.Helper()
	configPath, cleanup := setupOAuthTestEnv(t)
	h := NewHandler(configPath)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	return configPath, mux, cleanup
}

func getWorkspacesResponse(t *testing.T, mux *http.ServeMux) workspacesResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/workspaces status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp workspacesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

func putWorkspaces(t *testing.T, mux *http.ServeMux, payload workspacesPayload) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/workspaces", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rec, req)
	return rec
}

func TestHandleGetWorkspaces_ListsKeysWithoutLeakingSecrets(t *testing.T) {
	_, mux, cleanup := newWorkspacesTestServer(t)
	defer cleanup()

	resp := getWorkspacesResponse(t, mux)
	if resp.DefaultModel != "custom-default" {
		t.Fatalf("default model = %q, want custom-default", resp.DefaultModel)
	}
	if len(resp.Keys) != 1 {
		t.Fatalf("keys = %#v, want one entry", resp.Keys)
	}
	key := resp.Keys[0]
	wantFingerprint := config.APIKeyFingerprint("sk-default")
	if key.Fingerprint != wantFingerprint {
		t.Fatalf("fingerprint = %q, want %q", key.Fingerprint, wantFingerprint)
	}
	if key.ProfileID != "" {
		t.Fatalf("profile id = %q, want empty", key.ProfileID)
	}
	if key.KeyMask == "sk-default" || !strings.Contains(key.KeyMask, "****") {
		t.Fatalf("key mask %q should hide the secret", key.KeyMask)
	}
	if resp.ActiveWorkspace != resp.DefaultWorkspace {
		t.Fatalf("active workspace = %q, want default %q", resp.ActiveWorkspace, resp.DefaultWorkspace)
	}
	if len(resp.Agents) != 1 || resp.Agents[0].ID != "main" {
		t.Fatalf("agents = %#v, want implicit main agent", resp.Agents)
	}
	if resp.Agents[0].ProfileID != "" {
		t.Fatalf("agent profile = %q, want empty before binding", resp.Agents[0].ProfileID)
	}
}

func TestHandleUpdateWorkspaces_BindsKeyToIsolatedWorkspace(t *testing.T) {
	configPath, mux, cleanup := newWorkspacesTestServer(t)
	defer cleanup()

	isolated := filepath.Join(t.TempDir(), "team-a")
	restrict := true
	enabled := true
	payload := workspacesPayload{
		Enabled: &enabled,
		Profiles: []workspaceProfilePayload{
			{
				ID:                  "team-a",
				Name:                "Team A",
				Path:                isolated,
				RestrictToWorkspace: &restrict,
				APIKeyFingerprints:  []string{config.APIKeyFingerprint("sk-default")},
			},
		},
	}
	rec := putWorkspaces(t, mux, payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.WorkspaceIsolation == nil || len(cfg.WorkspaceIsolation.Profiles) != 1 {
		t.Fatalf("workspace isolation = %#v", cfg.WorkspaceIsolation)
	}
	if got := cfg.WorkspaceIsolation.Profiles[0].Path; got != isolated {
		t.Fatalf("saved path = %q, want %q", got, isolated)
	}

	resp := getWorkspacesResponse(t, mux)
	if len(resp.Keys) != 1 || resp.Keys[0].ProfileID != "team-a" {
		t.Fatalf("keys = %#v, want key bound to team-a", resp.Keys)
	}
	if resp.ActiveWorkspace != isolated {
		t.Fatalf("active workspace = %q, want %q", resp.ActiveWorkspace, isolated)
	}
	if resp.ActiveProfileID != "team-a" {
		t.Fatalf("active profile = %q, want team-a", resp.ActiveProfileID)
	}
	if len(resp.Agents) != 1 || resp.Agents[0].ProfileID != "team-a" || resp.Agents[0].Workspace != isolated {
		t.Fatalf("agents = %#v, want main bound to team-a %q", resp.Agents, isolated)
	}
}

func TestHandleUpdateWorkspaces_PrunesUnknownFingerprints(t *testing.T) {
	configPath, mux, cleanup := newWorkspacesTestServer(t)
	defer cleanup()

	enabled := true
	payload := workspacesPayload{
		Enabled: &enabled,
		Profiles: []workspaceProfilePayload{
			{
				ID:                 "stale",
				Path:               filepath.Join(t.TempDir(), "stale"),
				APIKeyFingerprints: []string{config.APIKeyFingerprint("sk-gone")},
			},
		},
	}
	rec := putWorkspaces(t, mux, payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body=%s", rec.Code, rec.Body.String())
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.WorkspaceIsolation == nil || len(cfg.WorkspaceIsolation.Profiles) != 1 {
		t.Fatalf("workspace isolation = %#v", cfg.WorkspaceIsolation)
	}
	if got := len(cfg.WorkspaceIsolation.Profiles[0].APIKeyFingerprints); got != 0 {
		t.Fatalf("fingerprints = %d, want 0 after prune", got)
	}
}

func TestHandleUpdateWorkspaces_RejectsInvalidPayload(t *testing.T) {
	_, mux, cleanup := newWorkspacesTestServer(t)
	defer cleanup()

	duplicateBase := filepath.Join(t.TempDir(), "same")
	tests := []struct {
		name    string
		payload workspacesPayload
		want    string
	}{
		{
			name: "relative path",
			payload: workspacesPayload{
				Profiles: []workspaceProfilePayload{{ID: "bad", Path: "relative/path"}},
			},
			want: "must be absolute",
		},
		{
			name: "duplicate paths",
			payload: workspacesPayload{
				Profiles: []workspaceProfilePayload{
					{ID: "a", Path: duplicateBase},
					{ID: "b", Path: duplicateBase},
				},
			},
			want: "duplicates workspace",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := putWorkspaces(t, mux, tc.payload)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body=%s", rec.Code, rec.Body.String())
			}
			var body struct {
				Status string   `json:"status"`
				Errors []string `json:"errors"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error body: %v", err)
			}
			if body.Status != "validation_error" {
				t.Fatalf("status = %q, want validation_error", body.Status)
			}
			if !strings.Contains(strings.Join(body.Errors, "\n"), tc.want) {
				t.Fatalf("errors = %v, want %q", body.Errors, tc.want)
			}
		})
	}
}
