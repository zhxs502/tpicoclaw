# 工作区隔离

> 返回 [指南目录](README.md)

工作区隔离让每个对话选择一个命名工作区目录。先在配置里定义 **名称 → 路径**
映射，然后在 Web UI 对话页的模型选择器旁选一个；同一个模型就能使用不同的文件、
会话和技能。

## 配置

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

| 字段 | 含义 |
| --- | --- |
| `enabled` | 是否启用工作区选择器；省略时，`profiles` 非空即启用。 |
| `profiles[].id` | 稳定标识，格式 `[A-Za-z0-9][A-Za-z0-9_-]{0,63}`；Web UI 会按名称生成。 |
| `profiles[].name` | Web UI 和对话选择器里显示的名称。 |
| `profiles[].path` | 工作区目录；绝对路径或以 `~` 开头，首次使用时创建。 |
| `profiles[].restrict_to_workspace` | 可选，覆盖该工作区对话的全局限制。 |
| `profiles[].allow_read_outside_workspace` | 可选，覆盖是否允许读目录外。 |

## Web UI 操作

1. 打开 **工作区**，启用工作区选择；
2. 新建工作区：名称 + 绝对路径，改动自动保存；
3. 黄色条出现后点 **重启服务**（或开启 `gateway.hot_reload`），让网关重新加载；
4. 到 **对话** 页，在模型选择器旁选择工作区。切换工作区会新建对话，历史分开。

不选工作区时，对话使用 `agents.defaults.workspace`。

## 运行时语义

- Web UI 在 pico `message.send` 的 payload 里带 `workspace: <id>`；
- 网关为这一轮使用选中工作区的 **agent 变体**：模型、人格和工具策略与路由到的
  agent 相同，但文件工具、`exec` 工作目录、`sessions/`、`skills/` 和记忆都使用
  该目录；
- 会话 key 不变，不同工作区的会话分别存放在各自的 `sessions/` 目录；
- 工作区配置在网关启动时读取，新增/修改后需要重启（或开热重载）；
- 新工作区默认是空目录，只有放了自己的 `AGENT.md`、`SOUL.md`、`skills/` 才会
  覆盖继承来的定义。

## 隔离边界

`restrict_to_workspace` / `allow_read_outside_workspace` 控制该工作区对话的文件
与命令工具权限，不是操作系统沙箱；子进程隔离仍由 `pkg/isolation` 负责。全局
`tools.allow_read_paths` / `tools.allow_write_paths` 依然生效。

## 校验规则

出现以下情况会拒绝保存：

- `id` 缺失、格式不对或重复；
- `path` 缺失、相对路径、重复，或嵌套在另一个工作区里；
- 工作区数量超过 64 个。

## 重启行为

选择器和网关的 agent 注册表都来自网关启动时加载的配置。保存工作区改动后 Web UI
会显示重启提示。也可以向网关健康端点 `POST /reload` 手动重载；开启
`gateway.hot_reload` 则自动重载。