---
slug: agentdesk
version: v0.1.0
materials_status: current
task_source: acceptance_criteria
---

# Tasks: agentdesk v0.1

## Task Ledger

- [x] TASK-001 仓库骨架、MIT、.gitignore、.env.example（AC-010）· ac=AC-010· evidence=TestOpenSourceFilesPresent
- [ ] TASK-002 provider 接口、事件类型、OpenAI 兼容 SSE 客户端、注册表（AC-001）· ac=AC-001· evidence=TestRegistryListsProviders
- [ ] TASK-003 流式解析：reasoning / content 分事件、usage、DONE（AC-002）· ac=AC-002· evidence=TestStreamSplitsReasoningAndContent
- [ ] TASK-004 工具注册表、JSON Schema、审批开关、内置工具（AC-008）· ac=AC-008· evidence=TestUnregisteredToolRejected
- [ ] TASK-005 多轮 agent loop 与迭代上限（AC-003）· ac=AC-003· evidence=TestLoopExecutesToolAndConverges
- [ ] TASK-006 SQLite 会话与消息持久化（AC-006）· ac=AC-006· evidence=TestSessionPersistsAcrossReopen
- [ ] TASK-007 CLI：run / providers / sessions / history / smoke（AC-005）· ac=AC-005· evidence=TestRunCommandStreams
- [ ] TASK-008 Wails v2 外壳与前端流式聊天（AC-004）· ac=AC-004· evidence=TestStreamEventsWired
- [ ] TASK-009 密钥装载与仓库卫生校验（AC-007）· ac=AC-007· evidence=TestSecretsNeverCommitted
- [ ] TASK-010 真实提供商 smoke 与证据登记（AC-009）· ac=AC-009· evidence=TestLiveProviders

## Boundary

Checkboxes are AC-bound ledgers. `[x]` is valid only when the bound evidence exists (completion ≠ self-tick).
