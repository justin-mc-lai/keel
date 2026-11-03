---
slug: agentdesk
version: v0.1.0
status: frozen
language: zh
domain_required: false
ux_required: false
parser_contract: beacon-feature-package-v2
truth_source_model: feature_package_authoritative
package_maturity: draft
materials_status: current
intent_confirmed: true
canonical_refs:
  truth: docs/beacon/v0.1.0/features/agentdesk/truth.md
  tests: docs/beacon/v0.1.0/features/agentdesk/tests.md
  tasks: docs/beacon/v0.1.0/features/agentdesk/tasks.md
  evidence: docs/beacon/v0.1.0/features/agentdesk/evidence.md
---

# Requirement Truth: agentdesk

## 人话

一个用 Go + Wails v2 写的桌面 LLM Agent 底座：单二进制、不依赖 Node/Python/Electron，换模型提供商只改一行配置，token 走进程内事件流而不是本地 HTTP。v0.1 要能装上就用、能真的调通我自己的两个提供商（OpenCode Go、Command Code），并且能开源给别人复刻。

## User Intent Snapshot

```yaml
lake_or_ocean: 湖
language: zh
source: user_intent
utterance: "自研一个桌面类 llm agent 进行开源，对话类，流传输，token 流式等基础 agent go 底座，使用 wails2 的单二进制应用"
goal: "交付一个可运行、可验证、可开源的 Go 桌面 Agent 底座 v0.1：provider 无关、token 流式、工具调用闭环、单二进制"
non_goals: "多智能体编排；MCP 客户端；图像/语音；厂商原生协议（responses/anthropic wire）；Windows/Linux 构建产物"
constraints: "MIT 许可；仅依赖 OpenAI 兼容 chat/completions；密钥不得入库；必须用真实提供商调用验证闭环"
scope_mode: focused
```

## Open Questions

| Q ID | Question | Recommendation | Status | Answer |
|------|----------|----------------|--------|--------|
| Q1 | v0.1 的范围切到哪里算够？ | 只做「provider 无关 + 流式 + 工具调用 + 持久化 + 单二进制」，其余延后 | closed | 用户确认按此范围交付；多智能体/MCP/多模态全部延后到 v0.2 |
| Q2 | 用哪些提供商验证闭环？ | 用用户现有 key：OpenCode Go 与 Command Code | closed | 两个都接，且必须真实调用成功才算通过 |
| Q3 | 传输层用 SSE 还是进程内事件？ | 桌面端用 Wails EventsEmit；provider 层保留 channel 以便后续 headless 复用 | closed | 按 channel + EventsEmit 实现，不引入本地 HTTP server |

## Intent Coverage

| AC ID | Intent Source | Evidence |
|-------|---------------|----------|
| AC-001 | utterance: provider 无关 go 底座 | conversation: 用户要求「llm提供商类的」 |
| AC-002 | utterance: 流传输，token 流式 | 探针实测 opencode.ai/zen/go/v1 返回 `delta.content` 增量 |
| AC-003 | 底座必须能真干活 | 探针实测 tool_calls 返回 `finish_reason=tool_calls` |
| AC-004 | utterance: wails2 的单二进制应用 | wails v2 官方文档 |
| AC-005 | 可开源给别人复刻 | 无 GUI 也必须能跑，便于 CI 与验收 |
| AC-006 | 对话类应用 | 会话必须能活过重启 |
| AC-007 | 安全边界 | 密钥只从环境/本地文件读，仓库零密钥 |
| AC-008 | 工具调用闭环 | 工具需声明 schema 与审批开关 |
| AC-009 | 可验收交付 | 必须有真实调用证据，不接受「应该能跑」 |
| AC-010 | 开源 | MIT + README + GitHub 仓库 |

## Acceptance Criteria

| AC ID | Criterion |
|-------|-----------|
| AC-001 | 存在 `provider.Provider` 接口与注册表；配置可声明多个 provider；`agentdesk providers` 列出 provider 与模型，其中 `opencode-go` 与 `commandcode` 两个条目均可完成一次流式调用 |
| AC-002 | 流式响应按 token 增量产出事件，且 `reasoning_content` 与 `content` 分为两类事件，不混进同一条正文字段 |
| AC-003 | Agent loop 支持「模型请求工具 → 本地执行 → 结果回填 → 继续生成」，有最大迭代上限；单测覆盖一次完整工具回合 |
| AC-004 | `wails build` 产出单个可执行文件；GUI 聊天界面能实时显示流式 token |
| AC-005 | `agentdesk run -p "..."` 无 GUI 可用，流式打印回答，成功退出码 0 |
| AC-006 | 会话与消息写入 SQLite（纯 Go 驱动，无 CGO）；进程重启后能读回历史 |
| AC-007 | 仓库中不含任何真实密钥；`.env.example` 列出所需变量；`.env*` 与 `*.db` 被 gitignore |
| AC-008 | 工具以 JSON Schema 声明；工具执行受审批开关控制，默认拒绝未登记工具 |
| AC-009 | `go test ./...` 全绿；`agentdesk smoke` 对两个真实提供商各完成一次流式调用并输出可核验摘要 |
| AC-010 | 仓库含 MIT LICENSE、README（安装/构建/配置/快速开始）、可推送到 GitHub 公开仓库 |

## 边界（v0.1 明确不做）

- 不做多 Agent 编排、不做 worktree 隔离
- 不做 MCP 客户端、不做 A2A
- 不做图像 / 语音 / 嵌入
- 不支持厂商原生 wire（OpenAI Responses、Anthropic Messages）——只走 OpenAI 兼容 `chat/completions`
- 只在 macOS arm64 验证构建产物；Windows/Linux 保证源码可编译但不产出安装包

## Parity Matrix

| Capability | Covered | Evidence |
|------------|---------|----------|
| provider 无关抽象 | yes | AC-001 |
| token 流式 | yes | AC-002 |
| 工具调用 | yes | AC-003 |
| 桌面单二进制 | yes | AC-004 |
| 无 GUI 可运行 | yes | AC-005 |
| 会话持久化 | yes | AC-006 |
