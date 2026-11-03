---
slug: agentdesk-permissions
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
  truth: docs/beacon/v0.1.0/features/agentdesk-permissions/truth.md
  tests: docs/beacon/v0.1.0/features/agentdesk-permissions/tests.md
  tasks: docs/beacon/v0.1.0/features/agentdesk-permissions/tasks.md
  evidence: docs/beacon/v0.1.0/features/agentdesk-permissions/evidence.md
---

# Requirement Truth: agentdesk-permissions

## 人话

桌面端的工具审批现在是写死的「一律拒绝」，界面上没有地方点同意，所以需要审批的工具在 GUI 里永远跑不起来。这个包补两件事：像 Codex / Claude 那样的三档权限选择（每次都问、本会话放行、全部放行），以及一个可选的思考强度下拉。

## User Intent Snapshot

```yaml
lake_or_ocean: 湖
language: zh
source: user_intent
utterance: "是需要有权限选择，正常是每次都问，或者直接权限允许，bypass 之类，需要的是参考 codex 、claude 类似的权限选择"
goal: "桌面端提供三档权限模式与可用的审批交互，并补上思考强度透传；两个湖同一个版本交付"
non_goals: "沙箱；按目录/命令的规则引擎；按工具的永久允许列表；厂商差异化的思考强度映射"
constraints: "默认必须是每次都问；bypass 必须有持续可见的警示；审批阻塞不能无限等待；不改动既有 10 条 AC 的行为"
scope_mode: focused
```

## Open Questions

| Q ID | Question | Recommendation | Status | Answer |
|------|----------|----------------|--------|--------|
| Q1 | 点「允许」的作用域是哪一个？ | 本会话内对该工具生效，卡片给「允许一次」「本会话允许」「拒绝」三个按钮 | closed | 用户确认按此实现 |
| Q2 | 两个湖是否同版交付？ | 同版，权限优先，思考强度独立可回滚 | closed | 用户要求两个湖统一规划一起进行 |

## Intent Coverage

| AC ID | Intent Source | Evidence |
|-------|---------------|----------|
| AC-011 | utterance: 每次都问 / 直接权限允许 / bypass | tool.go 现有 Approver 只有同步布尔，无作用域概念 |
| AC-012 | utterance: 参考 codex、claude 类似的权限选择 | desktop/app.go 写死 return false，UI 无处批准 |
| AC-013 | 约束: 审批阻塞不能无限等待 | agent loop 在流式 goroutine 内同步阻塞 |
| AC-014 | utterance: bypass 之类 | 开源默认值必须可见，避免静默全放行 |
| AC-015 | 用户上一轮追问「思考强度」 | Request 无 ReasoningEffort 字段 |

## Acceptance Criteria

| AC ID | Criterion |
|-------|-----------|
| AC-011 | 权限模式有三档 ask / allow / bypass 且默认 ask；allow 档下同一会话对同一工具只询问一次，新会话重新询问 |
| AC-012 | ask 档下需审批工具先产出审批请求事件并阻塞，收到决议后继续；拒绝时该工具结果为 error: tool: approval denied 且对话继续 |
| AC-013 | 审批请求 120 秒无响应自动判为拒绝并产出 denied 事件；Cancel 会话时未决审批立即结束 |
| AC-014 | bypass 档下不产出审批请求，且返回的运行状态带可见警示标识 |
| AC-015 | 请求可携带 reasoning_effort（off / low / medium / high）；未选择时不发送该字段，选择后原样写入请求体 |

## 边界（本版明确不做）

- 不做沙箱：`bypass` 只影响「要不要问」，不影响工具能做什么
- 不做按目录 / 按命令的规则引擎
- 不做跨会话的永久允许列表
- 不做厂商差异化的思考强度映射，不认该字段的厂商由上游忽略
- 不改动 agentdesk v0.1 已冻结的 10 条 AC

## Parity Matrix

| Capability | Covered | Evidence |
|------------|---------|----------|
| 三档权限 | yes | AC-011 |
| 审批交互可用 | yes | AC-012 |
| 阻塞有边界 | yes | AC-013 |
| bypass 可见 | yes | AC-014 |
| 思考强度透传 | yes | AC-015 |
