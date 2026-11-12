# Workspace Isolation

> Back to [Guides](README.md)

Workspace isolation lets each chat pick a named workspace directory. Define a
**name → path** mapping once, then choose it next to the model selector in the
Web UI. The same model can therefore run with different files, sessions and
skills.

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
        "allow_read_outside_workspace": false
      }
    ]
  }
}
```

| Field | Meaning |
| --- | --- |
| `enabled` | Enables the workspace selector. When omitted, isolation is enabled as soon as `profiles` is not empty. |
| `profiles[].id` | Stable identifier (`[A-Za-z0-9][A-Za-z0-9_-]{0,63}`); generated from the name in the Web UI. |
| `profiles[].name` | Display name in the Web UI and chat selector. |
| `profiles[].path` | Workspace directory. Absolute, or starting with `~`; created on first use. |
| `profiles[].restrict_to_workspace` | Optional per-workspace override of the global setting. |
| `profiles[].allow_read_outside_workspace` | Optional per-workspace read-outside override. |

## Web UI

1. Open **Workspaces** and enable workspace selection.
2. Create a workspace: name + absolute path. Changes save automatically.
3. Restart the gateway from the yellow banner (or enable `gateway.hot_reload`)
   so the running gateway reloads the profiles.
4. In **Chat**, pick the workspace next to the model selector. Switching
   workspace starts a new chat so histories stay separate.

With no workspace selected, chat uses `agents.defaults.workspace`.

## Runtime Semantics

- The Web UI sends `workspace: <id>` in the pico `message.send` payload.
- The gateway runs that turn with a cached **workspace variant** of the routed
  agent: same model, persona and tool policy, but the selected directory is used
  for file tools, `exec` cwd, `sessions/`, `skills/` and memory.
- Session keys are unchanged; sessions from different workspaces are stored in
  different `sessions/` directories.
- Workspace profiles are read at gateway startup. Restart the gateway (or enable
  hot reload) after adding or editing them.
- A workspace starts empty. It only gets its own `AGENT.md`, `SOUL.md` or
  `skills/` if you put files there; the agent definition is inherited from the
  routed agent.

## Isolation Limits

`restrict_to_workspace` / `allow_read_outside_workspace` control the file and
exec tools for chats using that workspace. This is not an OS sandbox:
`pkg/isolation` handles child-process confinement separately. Global
`tools.allow_read_paths` / `tools.allow_write_paths` still apply.

## Validation Rules

The configuration is rejected when:

- an `id` is missing, malformed or duplicated;
- a `path` is missing, relative, duplicated, or nested inside another workspace;
- more than 64 workspaces are configured.

## Restart Behavior

The selector and gateway agent registry are built from the config loaded at
gateway startup. The Web UI shows a restart banner after saving workspace
changes. `POST /reload` on the gateway health endpoint performs a manual reload
without a process restart; `gateway.hot_reload` enables automatic reload.