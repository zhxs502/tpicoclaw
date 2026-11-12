# Workspace Isolation

> Back to [Guides](README.md)

Workspace isolation binds model **API keys** to separate workspace directories.
An agent whose primary model uses a bound key reads, writes, executes commands
and stores sessions inside that directory instead of the shared default
workspace.

The feature is optional and disabled by default. It is configured either in the
Web UI (**Workspaces**) or directly in `config.json`.

## Configuration

```json
{
  "workspace_isolation": {
    "enabled": true,
    "profiles": [
      {
        "id": "team-a",
        "name": "Team A",
        "path": "~/.picoclaw/workspaces/team-a",
        "restrict_to_workspace": true,
        "allow_read_outside_workspace": false,
        "api_key_fingerprints": ["sha256:3f6a1c0d9e42b7a5"]
      }
    ]
  }
}
```

Fields:

| Field | Meaning |
| --- | --- |
| `enabled` | Turns the feature on. When omitted, isolation is enabled as soon as `profiles` is not empty. |
| `profiles[].id` | Stable identifier (`[A-Za-z0-9][A-Za-z0-9_-]{0,63}`). The Web UI generates one from the name. |
| `profiles[].name` | Display name in the Web UI. |
| `profiles[].path` | Workspace directory. Absolute, or starting with `~`. Created automatically. |
| `profiles[].restrict_to_workspace` | Optional per-profile override of `agents.defaults.restrict_to_workspace`. |
| `profiles[].allow_read_outside_workspace` | Optional per-profile override of `agents.defaults.allow_read_outside_workspace`. |
| `profiles[].api_key_fingerprints` | API keys routed to this workspace. Not the keys themselves — see below. |

### API key fingerprints

The config never stores raw API keys in this section. Each binding is a
truncated SHA-256 of the resolved key value, for example
`sha256:3f6a1c0d9e42b7a5`. Fingerprints are stable when a key is stored through
`enc://` or `file://` references, and become stale only when the key value
itself changes.

The Web UI computes fingerprints from the configured models and lets you select
a workspace per key. Assignments for keys that no longer exist are pruned on the
next save.

## Web UI

1. Open **Workspaces** in the sidebar.
2. Enable **Workspace isolation**.
3. Create a workspace: give it a name and an absolute path. The directory is
   created on the next agent start.
4. In **API key assignments**, pick the workspace for each API key.
5. Changes save automatically. Restart the gateway afterwards, or enable
   `gateway.hot_reload` so the config watcher rebuilds the agent registry
   automatically.

The **Agent workspaces** table shows where every configured agent will run. If
the API key of the model that actually handles your chat is not bound, chat
keeps using the default workspace even when other keys are bound — bind that
model's key, or switch the chat model, to isolate it.

The page also shows the **active workspace** for the current default chat model,
so you can verify which directory the chat is using.

## Runtime Semantics

- The workspace is resolved when an agent instance is created, from the API key
  of the agent's **primary model**. The **Active workspace** card reflects the
  default model; with dispatch rules the chat may be handled by another agent —
  check the **Agent workspaces** table for the effective directory.
- One model entry with several keys is expanded into `name` (first key) and
  `name__key_1`, `name__key_2`, ... entries; each expanded entry can be bound
  independently.
- Aliases used for load balancing (several `model_list` entries with the same
  `model_name`) are resolved deterministically from the first entry. Bind all
  keys of such an alias to the same workspace, otherwise the remaining entries
  are ignored for workspace selection.
- Unbound keys, local models without keys, and disabled isolation fall back to
  `agents.defaults.workspace`.
- When the effective workspace changes, the agent reloads `AGENT.md`, `SOUL.md`,
  `skills/`, session storage and file tools from the isolated directory.
- `restrict_to_workspace` and `allow_read_outside_workspace` from the matched
  profile override the global defaults for that agent only. Global
  `tools.allow_read_paths` / `tools.allow_write_paths` still apply.

## What Is Isolated

| Resource | Bound agent | Unbound agent |
| --- | --- | --- |
| File read/write/list tools | Isolated workspace | Default workspace |
| `exec` working directory | Isolated workspace | Default workspace |
| Sessions (`sessions/`) | Isolated workspace | Default workspace |
| Skills | Isolated workspace `skills/` | Default workspace `skills/` |
| Web UI session list / skills page | Follows the active workspace | Default workspace |

The following are process-wide and do not move per workspace:

- gateway logs
- cron and heartbeat state (stored at the default workspace)
- `pkg/isolation` child-process sandbox settings
- MCP servers configured globally

## Isolation Limits

Workspace isolation separates **agents and their file tools**. It is not an OS
sandbox:

- an unrestricted agent can still read other directories;
- child processes started by `exec` use the workspace as their working
  directory but are not chrooted — use [`pkg/isolation`](../../pkg/isolation/README.md)
  for process-level confinement;
- the Web UI itself runs with the launcher's permissions.

Enable `restrict_to_workspace` on the profile if you want the file and exec
tools to stay inside the directory.

## Validation Rules

The configuration is rejected when:

- an `id` is missing, malformed or duplicated;
- a `path` is missing, relative, duplicated, or nested inside another profile;
- a fingerprint is malformed or bound to more than one workspace;
- more than 64 profiles are configured.

## Restart Behavior

The gateway rebuilds its agent registry on config changes only when
`gateway.hot_reload` is enabled. Otherwise restart the gateway after saving —
the Web UI shows a restart-required notice when the running gateway still uses
the old configuration. `POST /reload` on the gateway health endpoint performs a
manual reload without a restart.
