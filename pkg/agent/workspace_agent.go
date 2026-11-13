// PicoClaw - Ultra-lightweight personal AI agent

package agent

import (
	"strings"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// metadataKeyWorkspace is the inbound metadata field used by channels (and the
// Web UI) to select one of the configured workspace profiles for a chat turn.
const metadataKeyWorkspace = "workspace"

// resolveWorkspaceAgent returns a cached variant of base that uses the workspace
// selected in the inbound context. When no workspace (or an unknown one) is
// selected, the base agent is returned unchanged.
func (al *AgentLoop) resolveWorkspaceAgent(base *AgentInstance, ctx *bus.InboundContext) *AgentInstance {
	if base == nil || ctx == nil || len(ctx.Raw) == 0 {
		return base
	}
	workspaceID := strings.TrimSpace(ctx.Raw[metadataKeyWorkspace])
	if workspaceID == "" {
		return base
	}

	cfg := al.GetConfig()
	if cfg == nil || cfg.WorkspaceIsolation == nil || !cfg.WorkspaceIsolation.IsEnabled() {
		return base
	}
	path, profile, ok := cfg.WorkspacePathByID(workspaceID)
	if !ok || profile == nil {
		return base
	}

	cacheKey := base.ID + "\x00" + profile.ID
	if cached, ok := al.workspaceAgents.Load(cacheKey); ok {
		if variant, ok := cached.(*AgentInstance); ok {
			return variant
		}
	}

	al.workspaceAgentsMu.Lock()
	defer al.workspaceAgentsMu.Unlock()
	if cached, ok := al.workspaceAgents.Load(cacheKey); ok {
		if variant, ok := cached.(*AgentInstance); ok {
			return variant
		}
	}

	agentCfg := &config.AgentConfig{
		ID:        base.ID,
		Name:      base.Name,
		Workspace: path,
		Model: &config.AgentModelConfig{
			Primary:   base.Model,
			Fallbacks: append([]string(nil), base.Fallbacks...),
		},
		Subagents: base.Subagents,
		Skills:    append([]string(nil), base.SkillsFilter...),
	}

	variant := newAgentInstance(
		agentCfg,
		&cfg.Agents.Defaults,
		cfg,
		base.Provider,
		agentInstanceOptions{
			Workspace:  path,
			ProfileID:  profile.ID,
			Definition: &base.Definition,
		},
	)

	// The base registry only knows the original agent; build a throwaway
	// registry so shared tools (web, message, spawn, ...) can be registered on
	// the variant exactly as they are for configured agents.
	tmpRegistry := &AgentRegistry{
		cfg:    cfg,
		agents: map[string]*AgentInstance{base.ID: variant},
	}
	registerSharedTools(al, cfg, al.bus, tmpRegistry, variant.Provider)
	if al.mediaStore != nil {
		variant.Tools.SetMediaStore(al.mediaStore)
	}
	if registry := al.GetRegistry(); registry != nil && variant.ContextBuilder != nil {
		variant.ContextBuilder.WithAgentDiscovery(base.ID, registry.ListSpawnableAgents)
	}

	al.workspaceAgents.Store(cacheKey, variant)
	logger.InfoCF("agent", "Created workspace agent variant",
		map[string]any{
			"agent_id":     base.ID,
			"workspace_id": profile.ID,
			"workspace":    path,
		})
	return variant
}

// closeWorkspaceAgents releases session stores held by cached variants.
func (al *AgentLoop) closeWorkspaceAgents() {
	al.workspaceAgentsMu.Lock()
	defer al.workspaceAgentsMu.Unlock()
	al.workspaceAgents.Range(func(key, value any) bool {
		if variant, ok := value.(*AgentInstance); ok {
			if err := variant.Close(); err != nil {
				logger.WarnCF("agent", "Failed to close workspace agent",
					map[string]any{"error": err.Error()})
			}
		}
		al.workspaceAgents.Delete(key)
		return true
	})
}
