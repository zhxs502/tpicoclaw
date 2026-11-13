package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// workspaceProfileResponse is the Web UI representation of one workspace.
type workspaceProfileResponse struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Path string `json:"path"`
}

type workspacesResponse struct {
	Enabled          bool                       `json:"enabled"`
	DefaultWorkspace string                     `json:"default_workspace"`
	Profiles         []workspaceProfileResponse `json:"profiles"`
}

type workspaceProfilePayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type workspacesPayload struct {
	Enabled  *bool                     `json:"enabled"`
	Profiles []workspaceProfilePayload `json:"profiles"`
}

// registerWorkspaceRoutes binds the workspace selection endpoints.
func (h *Handler) registerWorkspaceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/workspaces", h.handleGetWorkspaces)
	mux.HandleFunc("PUT /api/workspaces", h.handleUpdateWorkspaces)
}

// handleGetWorkspaces returns the named workspace profiles.
//
//	GET /api/workspaces
func (h *Handler) handleGetWorkspaces(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSONResponse(w, buildWorkspacesResponse(cfg))
}

// handleUpdateWorkspaces replaces the workspace profiles.
//
//	PUT /api/workspaces
func (h *Handler) handleUpdateWorkspaces(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var payload workspacesPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	cfg, err := config.LoadConfig(h.configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	isolation := &config.WorkspaceIsolationConfig{
		Enabled:  payload.Enabled,
		Profiles: make([]config.WorkspaceProfileConfig, 0, len(payload.Profiles)),
	}
	for _, profile := range payload.Profiles {
		isolation.Profiles = append(isolation.Profiles, config.WorkspaceProfileConfig{
			ID:   profile.ID,
			Name: profile.Name,
			Path: profile.Path,
		})
	}

	config.NormalizeWorkspaceIsolation(isolation)

	if errs := isolation.Validate(); len(errs) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"status": "validation_error",
			"errors": errs,
		})
		return
	}

	if len(isolation.Profiles) == 0 && (isolation.Enabled == nil || !*isolation.Enabled) {
		cfg.WorkspaceIsolation = nil
	} else {
		cfg.WorkspaceIsolation = isolation
	}

	if err := config.SaveConfig(h.configPath, cfg); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	logger.Infof("workspace profiles updated")

	writeJSONResponse(w, buildWorkspacesResponse(cfg))
}

func writeJSONResponse(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func buildWorkspacesResponse(cfg *config.Config) workspacesResponse {
	resp := workspacesResponse{
		Profiles: make([]workspaceProfileResponse, 0),
	}
	if cfg == nil {
		return resp
	}

	resp.Enabled = cfg.WorkspaceIsolation.IsEnabled()
	resp.DefaultWorkspace = cfg.DefaultWorkspacePath()

	if cfg.WorkspaceIsolation != nil {
		for _, profile := range cfg.WorkspaceIsolation.Profiles {
			resp.Profiles = append(resp.Profiles, workspaceProfileResponse{
				ID:   profile.ID,
				Name: profile.Name,
				Path: profile.Path,
			})
		}
	}
	return resp
}

// workspaceSessionsDir resolves the sessions directory for a workspace id. An
// empty or unknown id falls back to the default workspace.
func workspaceSessionsDir(cfg *config.Config, workspaceID string) string {
	if cfg == nil {
		return resolveSessionsDir("")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID != "" {
		if path, _, ok := cfg.WorkspacePathByID(workspaceID); ok {
			return resolveSessionsDir(path)
		}
	}
	return resolveSessionsDir(cfg.DefaultWorkspacePath())
}
