# Codex app-server JSON-RPC 接入

codex-go 通过 `codex app-server --listen stdio://` 启动常驻 Codex 子进程，并使用换行分隔的 JSON-RPC 消息通信。

## 会话生命周期

1. `initialize`：声明 `codex-go` 客户端和 `experimentalApi` 能力。
2. `initialized`：初始化完成通知。
3. `thread/start` 或 `thread/resume`：创建或接管 Codex thread。
4. `turn/start`：发送用户消息。
5. 监听 `item/agentMessage/delta`、`item/started`、`item/completed`、`turn/completed` 等通知并映射到内部事件。

## 权限请求

以下 server request 会映射到现有微信和 Web 权限审批流：

- `item/commandExecution/requestApproval`
- `item/fileChange/requestApproval`
- `item/tool/requestUserInput`

批准时返回 `{"decision":"accept"}`，拒绝时返回 `{"decision":"decline"}`。用户输入请求返回 `{"answers":{...}}`。

## 部署

Linux 部署使用 systemd 管理 codex-go 主进程。codex-go 主进程再按会话启动和停止 `codex app-server` 子进程。
