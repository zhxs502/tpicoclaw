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

func TestHandleGetWorkspaces_EmptyProfiles(t *testing.T) {
	_, mux, cleanup := newWorkspacesTestServer(t)
	defer cleanup()

	resp := getWorkspacesResponse(t, mux)
	if resp.Enabled {
		t.Fatal("isolation should be disabled without profiles")
	}
	if len(resp.Profiles) != 0 {
		t.Fatalf("profiles = %#v, want empty", resp.Profiles)
	}
	if resp.DefaultWorkspace == "" {
		t.Fatal("default workspace should be resolved")
	}
}

func TestHandleUpdateWorkspaces_SavesNamePathMapping(t *testing.T) {
	configPath, mux, cleanup := newWorkspacesTestServer(t)
	defer cleanup()

	isolated := filepath.Join(t.TempDir(), "team-a")
	enabled := true
	payload := workspacesPayload{
		Enabled: &enabled,
		Profiles: []workspaceProfilePayload{
			{
				ID:   "team-a",
				Name: "Team A",
				Path: isolated,
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
	profile := cfg.WorkspaceIsolation.Profiles[0]
	if profile.Path != isolated || profile.Name != "Team A" {
		t.Fatalf("saved profile = %#v", profile)
	}

	resp := getWorkspacesResponse(t, mux)
	if !resp.Enabled || len(resp.Profiles) != 1 || resp.Profiles[0].ID != "team-a" {
		t.Fatalf("response = %#v", resp)
	}
	if resp.Profiles[0].Path != isolated {
		t.Fatalf("profile path = %q, want %q", resp.Profiles[0].Path, isolated)
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
