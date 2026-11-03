---
slug: agentdesk
version: v0.1.0
materials_status: current
---

# Tests: agentdesk v0.1

判定语义：Pass / Fail / Blocked。Blocked 只能用于外部依赖不可用（提供商网络、密钥缺失），且必须附命令输出。

| AC ID | TC ID | Layer | Command | Assertion |
|-------|-------|-------|---------|-----------|
| AC-001 | TC-001 | behavior | `go test ./internal/provider/ -run TestRegistryListsProviders` | 至少列出 opencode-go 与 commandcode，且各自带模型清单 |
| AC-002 | TC-002 | behavior | `go test ./internal/provider/ -run TestStreamSplitsReasoningAndContent` | reasoning 与 content 产出不同类型事件，`[DONE]` 收尾，usage 非空 |
| AC-003 | TC-003 | behavior | `go test ./internal/agent/ -run TestLoopExecutesToolAndConverges` | 假 provider 触发一次 tool_call，loop 执行后二次请求收敛为最终答案 |
| AC-004 | TC-004 | behavior | `go test ./desktop/ -run TestStreamEventsWired` | Wails 绑定把 delta 事件转发到前端事件名，构建配置指向单一入口 |
| AC-005 | TC-005 | behavior | `go test ./cmd/agentdesk/ -run TestRunCommandStreams` | 无 GUI 路径可运行，流式写出并返回 0 |
| AC-006 | TC-006 | behavior | `go test ./internal/store/ -run TestSessionPersistsAcrossReopen` | 关闭后重新打开仍能读回消息 |
| AC-007 | TC-007 | behavior | `go test ./internal/config/ -run TestSecretsNeverCommitted` | 仓库无密钥常量，`.env.example` 只含变量名 |
| AC-008 | TC-008 | behavior | `go test ./internal/tool/ -run TestUnregisteredToolRejected` | 未登记工具返回拒绝事件且不执行 |
| AC-009 | TC-009 | behavior | `go test ./internal/smoke/ -run TestLiveProviders` | both_providers_emit_non_empty_tokens |
| AC-010 | TC-010 | behavior | `go test ./internal/config/ -run TestOpenSourceFilesPresent` | LICENSE、README、.gitignore 存在且 LICENSE 为 MIT |
