package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// workspaceProfileResponse is the Web UI representation of one isolated workspace.
type workspaceProfileResponse struct {
	ID                        string   `json:"id"`
	Name                      string   `json:"name,omitempty"`
	Path                      string   `json:"path"`
	RestrictToWorkspace       *bool    `json:"restrict_to_workspace,omitempty"`
	AllowReadOutsideWorkspace *bool    `json:"allow_read_outside_workspace,omitempty"`
	APIKeyFingerprints        []string `json:"api_key_fingerprints"`
	KeyCount                  int      `json:"key_count"`
}

// workspaceKeyResponse describes one model API key and its binding.
type workspaceKeyResponse struct {
	ModelName      string `json:"model_name"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	KeyIndex       int    `json:"key_index"`
	KeyMask        string `json:"key_mask"`
	Fingerprint    string `json:"fingerprint"`
	ProfileID      string `json:"profile_id,omitempty"`
	IsDefaultModel bool   `json:"is_default_model"`
}

// agentWorkspaceResponse shows the effective workspace of one configured agent.
type agentWorkspaceResponse struct {
	ID                  string `json:"id"`
	Name                string `json:"name,omitempty"`
	Model               string `json:"model"`
	Workspace           string `json:"workspace"`
	ProfileID           string `json:"profile_id,omitempty"`
	IsDefault           bool   `json:"is_default"`
	RestrictToWorkspace bool   `json:"restrict_to_workspace"`
}

type workspacesResponse struct {
	Enabled          bool                       `json:"enabled"`
	DefaultWorkspace string                     `json:"default_workspace"`
	DefaultModel     string                     `json:"default_model"`
	ActiveWorkspace  string                     `json:"active_workspace"`
	ActiveProfileID  string                     `json:"active_profile_id,omitempty"`
	Profiles         []workspaceProfileResponse `json:"profiles"`
	Keys             []workspaceKeyResponse     `json:"keys"`
	Agents           []agentWorkspaceResponse   `json:"agents"`
	Warnings         []string                   `json:"warnings,omitempty"`
}

type workspaceProfilePayload struct {
	ID                        string   `json:"id"`
	Name                      string   `json:"name"`
	Path                      string   `json:"path"`
	RestrictToWorkspace       *bool    `json:"restrict_to_workspace"`
	AllowReadOutsideWorkspace *bool    `json:"allow_read_outside_workspace"`
	APIKeyFingerprints        []string `json:"api_key_fingerprints"`
}

type workspacesPayload struct {
	Enabled  *bool                     `json:"enabled"`
	Profiles []workspaceProfilePayload `json:"profiles"`
}

// registerWorkspaceRoutes binds the workspace isolation endpoints.
func (h *Handler) registerWorkspaceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/workspaces", h.handleGetWorkspaces)
	mux.HandleFunc("PUT /api/workspaces", h.handleUpdateWorkspaces)
}

// handleGetWorkspaces returns the workspace isolation configuration together
// with every configured model API key and its current binding.
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

// handleUpdateWorkspaces replaces the workspace isolation configuration.
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
			ID:                        profile.ID,
			Name:                      profile.Name,
			Path:                      profile.Path,
			RestrictToWorkspace:       profile.RestrictToWorkspace,
			AllowReadOutsideWorkspace: profile.AllowReadOutsideWorkspace,
			APIKeyFingerprints:        profile.APIKeyFingerprints,
		})
	}

	config.NormalizeWorkspaceIsolation(isolation)

	// Drop bindings for keys that no longer exist in model_list. The UI only
	// round-trips fingerprints returned by GET, but stale entries may survive
	// in the config after a key was rotated or removed.
	knownFingerprints := collectModelAPIKeyFingerprints(cfg)
	pruned := 0
	for i := range isolation.Profiles {
		kept := make([]string, 0, len(isolation.Profiles[i].APIKeyFingerprints))
		for _, fingerprint := range isolation.Profiles[i].APIKeyFingerprints {
			if _, ok := knownFingerprints[strings.ToLower(fingerprint)]; !ok {
				pruned++
				continue
			}
			kept = append(kept, fingerprint)
		}
		isolation.Profiles[i].APIKeyFingerprints = kept
	}
	if pruned > 0 {
		logger.WarnCF("workspaces", "Pruned workspace bindings for unknown api keys",
			map[string]any{"count": pruned})
	}

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

	logger.Infof("workspace isolation configuration updated")

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
		Keys:     make([]workspaceKeyResponse, 0),
	}
	if cfg == nil {
		return resp
	}

	resp.Enabled = cfg.WorkspaceIsolation.IsEnabled()
	resp.DefaultWorkspace = cfg.DefaultWorkspacePath()
	resp.DefaultModel = strings.TrimSpace(cfg.Agents.Defaults.GetModelName())

	boundFingerprints := make(map[string]struct{})
	if cfg.WorkspaceIsolation != nil {
		for _, profile := range cfg.WorkspaceIsolation.Profiles {
			item := workspaceProfileResponse{
				ID:                        profile.ID,
				Name:                      profile.Name,
				Path:                      profile.Path,
				RestrictToWorkspace:       profile.RestrictToWorkspace,
				AllowReadOutsideWorkspace: profile.AllowReadOutsideWorkspace,
				APIKeyFingerprints:        append([]string{}, profile.APIKeyFingerprints...),
				KeyCount:                  len(profile.APIKeyFingerprints),
			}
			resp.Profiles = append(resp.Profiles, item)
			for _, fingerprint := range profile.APIKeyFingerprints {
				boundFingerprints[strings.ToLower(strings.TrimSpace(fingerprint))] = struct{}{}
			}
		}
	}

	knownFingerprints := make(map[string]struct{})
	for _, modelCfg := range cfg.ModelList {
		if modelCfg == nil || modelCfg.IsVirtual() {
			continue
		}
		keys := modelCfg.APIKeys.Values()
		if len(keys) == 0 {
			continue
		}
		provider, modelID := providers.ExtractProtocol(modelCfg)
		for i, key := range keys {
			fingerprint := config.APIKeyFingerprint(key)
			if fingerprint == "" {
				continue
			}
			knownFingerprints[fingerprint] = struct{}{}
			profileID := workspaceProfileIDForFingerprint(cfg, fingerprint)
			resp.Keys = append(resp.Keys, workspaceKeyResponse{
				ModelName:      modelCfg.ModelName,
				Provider:       provider,
				Model:          modelID,
				KeyIndex:       i,
				KeyMask:        maskAPIKey(key),
				Fingerprint:    fingerprint,
				ProfileID:      profileID,
				IsDefaultModel: modelCfg.ModelName == resp.DefaultModel,
			})
		}
	}

	if cfg.WorkspaceIsolation != nil {
		stale := 0
		for fingerprint := range boundFingerprints {
			if _, ok := knownFingerprints[fingerprint]; !ok {
				stale++
			}
		}
		if stale > 0 {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf(
				"%d api key binding(s) no longer match a configured model and will be ignored.", stale))
		}
	}

	if resp.DefaultModel != "" {
		workspace, profile := cfg.ActiveWorkspaceForModel(resp.DefaultModel)
		resp.ActiveWorkspace = workspace
		if profile != nil {
			resp.ActiveProfileID = profile.ID
		}
	}
	if resp.ActiveWorkspace == "" {
		resp.ActiveWorkspace = resp.DefaultWorkspace
	}

	resp.Agents = buildAgentWorkspaces(cfg)

	return resp
}

// buildAgentWorkspaces reports where each configured agent will run once the
// gateway reloads. It mirrors pkg/agent.resolveAgentWorkspace and applies the
// same api key binding used by the runtime.
func buildAgentWorkspaces(cfg *config.Config) []agentWorkspaceResponse {
	result := make([]agentWorkspaceResponse, 0)
	if cfg == nil {
		return result
	}

	agents := cfg.Agents.List
	if len(agents) == 0 {
		agents = []config.AgentConfig{{ID: "main", Default: true}}
	}

	defaultModel := strings.TrimSpace(cfg.Agents.Defaults.GetModelName())
	for i := range agents {
		ac := &agents[i]
		id := strings.ToLower(strings.TrimSpace(ac.ID))
		if id == "" {
			id = "main"
		}
		model := defaultModel
		if ac.Model != nil && strings.TrimSpace(ac.Model.Primary) != "" {
			model = strings.TrimSpace(ac.Model.Primary)
		}
		workspace := agentWorkspacePath(cfg, ac)
		restrict := cfg.Agents.Defaults.RestrictToWorkspace
		profileID := ""
		if mc := cfg.PrimaryModelConfig(model); mc != nil {
			if path, profile := cfg.WorkspaceForModelConfig(mc); profile != nil {
				workspace = path
				profileID = profile.ID
				restrict = profile.EffectiveRestrict(restrict)
			}
		}
		result = append(result, agentWorkspaceResponse{
			ID:                  id,
			Name:                strings.TrimSpace(ac.Name),
			Model:               model,
			Workspace:           workspace,
			ProfileID:           profileID,
			IsDefault:           ac.Default,
			RestrictToWorkspace: restrict,
		})
	}
	return result
}

func agentWorkspacePath(cfg *config.Config, ac *config.AgentConfig) string {
	defaultWorkspace := cfg.DefaultWorkspacePath()
	if ac == nil {
		return defaultWorkspace
	}
	if strings.TrimSpace(ac.Workspace) != "" {
		return config.ExpandWorkspacePath(ac.Workspace)
	}
	id := strings.ToLower(strings.TrimSpace(ac.ID))
	if id == "" || id == "main" || ac.Default {
		return defaultWorkspace
	}
	return filepath.Join(defaultWorkspace, "..", "workspace-"+id)
}

func workspaceProfileIDForFingerprint(cfg *config.Config, fingerprint string) string {
	if cfg == nil || cfg.WorkspaceIsolation == nil {
		return ""
	}
	fingerprint = strings.ToLower(strings.TrimSpace(fingerprint))
	for i := range cfg.WorkspaceIsolation.Profiles {
		profile := &cfg.WorkspaceIsolation.Profiles[i]
		for _, bound := range profile.APIKeyFingerprints {
			if strings.EqualFold(strings.TrimSpace(bound), fingerprint) {
				return profile.ID
			}
		}
	}
	return ""
}

func collectModelAPIKeyFingerprints(cfg *config.Config) map[string]struct{} {
	result := make(map[string]struct{})
	if cfg == nil {
		return result
	}
	for _, modelCfg := range cfg.ModelList {
		if modelCfg == nil {
			continue
		}
		for _, key := range modelCfg.APIKeys.Values() {
			if fingerprint := config.APIKeyFingerprint(key); fingerprint != "" {
				result[fingerprint] = struct{}{}
			}
		}
	}
	return result
}

// effectiveWorkspacePath resolves the workspace used by the default chat agent.
// It follows the same api-key binding as the runtime so session and skill
// endpoints stay in sync with the isolated workspace.
func effectiveWorkspacePath(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	workspace, _ := cfg.ActiveWorkspaceForModel(cfg.Agents.Defaults.GetModelName())
	if strings.TrimSpace(workspace) != "" {
		return workspace
	}
	return cfg.DefaultWorkspacePath()
}
