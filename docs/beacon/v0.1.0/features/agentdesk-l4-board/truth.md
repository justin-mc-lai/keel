---
slug: agentdesk-l4-board
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
  truth: docs/beacon/v0.1.0/features/agentdesk-l4-board/truth.md
  tests: docs/beacon/v0.1.0/features/agentdesk-l4-board/tests.md
  tasks: docs/beacon/v0.1.0/features/agentdesk-l4-board/tasks.md
  evidence: docs/beacon/v0.1.0/features/agentdesk-l4-board/evidence.md
---

# Requirement Truth: agentdesk-l4-board

## 人话

L3 已经有工单、状态机、槽位、面板三问，但只有命令行能看到。这一湖把工单看板、单条历史、面板三问搬进界面，桌面与浏览器共用同一套页面。

## User Intent Snapshot

```yaml
lake_or_ocean: 湖（可视化第一片）
language: zh
source: user_intent
utterance: "现在想要的是支持桌面以及 web ui 的可视化"
goal: "工单看板、单条历史、面板三问进入 UI，两个外壳都能用"
non_goals: "DAG 编排（L5）；容器与指标（L6）；拖拽编辑器（L7）"
constraints: "界面数据必须来自 core，不得在前端自行计算状态；无工单时不得白屏"
scope_mode: focused
```

## Open Questions

| Q ID | Question | Recommendation | Status | Answer |
|------|----------|----------------|--------|--------|
| Q1 | 看板是否支持操作？ | 支持接受、拒绝、重试三个动作 | closed | 采用 |
| Q2 | 看板与聊天如何共存？ | 侧栏切换视图，共用同一外壳 | closed | 采用 |

## Intent Coverage

| AC ID | Intent Source | Evidence |
|-------|---------------|----------|
| AC-501 | 目标: 工单可见 | 现只有 `agentdesk tickets` 命令 |
| AC-502 | 目标: 看板可视化 | 前端无工单相关代码 |
| AC-503 | 目标: 面板三问可见 | 现只有 `agentdesk panel` 命令 |
| AC-504 | 目标: 人工介入 | FSM 已有 accept/reject/retry，界面未接 |
| AC-505 | 约束: 不得白屏 | 前端无空态与错误态 |

## Acceptance Criteria

| AC ID | Criterion |
|-------|-----------|
| AC-501 | core 暴露工单列表与工单动作两个方法；HTTP 适配器提供 `GET /api/tickets` 与 `POST /api/tickets/{id}/actions` |
| AC-502 | 界面有工单视图，按状态分组展示工单标题、来源与时间；点击某条可看到它的完整历史 |
| AC-503 | 界面展示面板三问：需要我处理的工单数量与列表、各状态计数与槽位占用、选中工单的历史 |
| AC-504 | 界面对 awaiting_review 的工单提供接受与拒绝，对 failed 的工单提供重试；动作成功后看板数据刷新 |
| AC-505 | 无工单时显示空态文案；接口失败时显示错误信息且页面仍可用 |

## 边界（本湖明确不做）

- 不做拖拽编排
- 不做工单新建（工单由 Trigger 产生）
- 不做实时推送刷新，用显式刷新
- 不做多用户并发编辑

## Parity Matrix

| Capability | Covered | Evidence |
|------------|---------|----------|
| 工单 API | yes | AC-501 |
| 看板与历史 | yes | AC-502 |
| 面板三问 | yes | AC-503 |
| 工单动作 | yes | AC-504 |
| 空态与错误态 | yes | AC-505 |
