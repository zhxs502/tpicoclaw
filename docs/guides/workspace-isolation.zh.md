# 工作区隔离

> 返回 [指南目录](README.md)

工作区隔离把模型的 **API Key** 绑定到独立的工作区目录。使用已绑定 Key 的智能体，其文件读写、命令执行与会话记录都会落在该目录里，而不是共享的默认工作区。

该功能默认关闭，可以在 Web UI 的 **工作区** 页配置，也可以直接改 `config.json`。

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
        "allow_read_outside_workspace": false,
        "api_key_fingerprints": ["sha256:3f6a1c0d9e42b7a5"]
      }
    ]
  }
}
```

字段说明：

| 字段 | 含义 |
| --- | --- |
| `enabled` | 是否启用。省略时，只要 `profiles` 非空就视为启用。 |
| `profiles[].id` | 稳定标识，格式 `[A-Za-z0-9][A-Za-z0-9_-]{0,63}`；Web UI 会按名称自动生成。 |
| `profiles[].name` | Web UI 中显示的名称。 |
| `profiles[].path` | 工作区目录；绝对路径，或以 `~` 开头。首次启动时自动创建。 |
| `profiles[].restrict_to_workspace` | 可选，按工作区覆盖 `agents.defaults.restrict_to_workspace`。 |
| `profiles[].allow_read_outside_workspace` | 可选，按工作区覆盖 `agents.defaults.allow_read_outside_workspace`。 |
| `profiles[].api_key_fingerprints` | 路由到该工作区的 API Key 指纹，不是 Key 本身，见下文。 |

### API Key 指纹

配置里不保存 Key 明文，只保存 Key 解析值的 SHA-256 截断指纹，例如
`sha256:3f6a1c0d9e42b7a5`。用 `enc://` 或 `file://` 引用保存的 Key 指纹也保持一致；只有 Key 值本身变了，绑定才会失效。

Web UI 会根据已配置的模型算出指纹，并允许逐个 Key 选择工作区；Model 列表里已经不存在的 Key，其绑定会在下次保存时被清理。

## Web UI 操作

1. 在侧边栏打开 **工作区**；
2. 打开 **启用工作区隔离**；
3. 新建工作区：填写名称与绝对路径，目录会在智能体下次启动时创建；
4. 在 **API Key 分配** 里，为每个 Key 选择工作区；
5. 改动会自动保存。随后重启网关，或者开启 `gateway.hot_reload`，让配置监听自动重建智能体注册表。

页面底部 **智能体工作区** 表会显示每个智能体重启后实际使用的目录。如果真正处理对话的模型（比如默认模型的）Key 没绑定，聊天还是会走默认工作区——把那个模型的 Key 绑上，或切换对话模型，才会隔离。

页面顶部还会显示当前默认对话模型实际使用的 **当前工作区**，方便核对对话落在哪个目录。

## 运行时语义

- 工作区在创建智能体实例时解析，依据是该智能体 **主模型** 的 API Key。「当前工作区」卡片只代表默认模型；有 dispatch 规则时对话可能由别的智能体处理，以页面底部 **智能体工作区** 表为准。
- 一个模型配置多个 Key 时，加载阶段会展开成 `name`（第一个 Key）和 `name__key_1`、`name__key_2`……每个展开项可以独立绑定。
- 负载均衡别名（多个 `model_list` 条目使用同一个 `model_name`）按第一个条目确定性地解析。请把同一别名的所有 Key 绑到同一个工作区，否则其余条目不会参与工作区选择。
- 未绑定的 Key、没有 Key 的本地模型、关闭隔离时，都回落到 `agents.defaults.workspace`。
- 生效工作区变化时，智能体会从隔离目录重新加载 `AGENT.md`、`SOUL.md`、`skills/`、会话存储与文件工具。
- 命中的 profile 里的 `restrict_to_workspace` / `allow_read_outside_workspace` 只覆盖该智能体；全局的 `tools.allow_read_paths` / `tools.allow_write_paths` 仍然生效。

## 隔离范围

| 资源 | 已绑定智能体 | 未绑定智能体 |
| --- | --- | --- |
| 文件读/写/列目录工具 | 隔离工作区 | 默认工作区 |
| `exec` 工作目录 | 隔离工作区 | 默认工作区 |
| 会话（`sessions/`） | 隔离工作区 | 默认工作区 |
| 技能 | 隔离工作区的 `skills/` | 默认工作区 `skills/` |
| Web UI 会话列表 / 技能页 | 跟随当前工作区 | 默认工作区 |

以下内容仍是进程级的，不随工作区变化：

- 网关日志
- cron 与心跳状态（存放在默认工作区）
- `pkg/isolation` 的子进程沙箱设置
- 全局配置的 MCP 服务

## 隔离边界

工作区隔离隔离的是 **智能体及其文件工具**，不是操作系统沙箱：

- 未开启限制的智能体仍可读取其他目录；
- `exec` 启动的子进程以工作区为当前目录，但不会被 chroot；需要进程级限制请用 [`pkg/isolation`](../../pkg/isolation/README.zh.md)；
- Web UI 本身仍以 launcher 的权限运行。

如果希望文件与命令工具严格留在目录内，请在工作区上开启 `restrict_to_workspace`。

## 校验规则

出现以下情况会拒绝保存：

- `id` 缺失、格式不对或重复；
- `path` 缺失、是相对路径、重复，或嵌套在另一个工作区里；
- 指纹格式不对，或同一个 Key 绑定到多个工作区；
- 工作区数量超过 64 个。

## 重启行为

只有开启 `gateway.hot_reload` 时，网关才会在配置变化后自动重建智能体注册表。否则请在保存后重启网关——如果运行中的网关仍在用旧配置，Web UI 会提示需要重启。也可以向网关健康端点 `POST /reload` 手动重载，无需重启进程。
