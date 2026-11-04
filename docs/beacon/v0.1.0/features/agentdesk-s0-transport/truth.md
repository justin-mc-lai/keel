---
slug: agentdesk-s0-transport
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
  truth: docs/beacon/v0.1.0/features/agentdesk-s0-transport/truth.md
  tests: docs/beacon/v0.1.0/features/agentdesk-s0-transport/tests.md
  tasks: docs/beacon/v0.1.0/features/agentdesk-s0-transport/tasks.md
  evidence: docs/beacon/v0.1.0/features/agentdesk-s0-transport/evidence.md
---

# Requirement Truth: agentdesk-s0-transport

## 人话

今天界面和 Wails 焊死了：绑定是结构体方法，事件是桌面 API。要同时出桌面和浏览器两套外壳，必须先有一层与传输无关的服务，再挂两个适配器。不先做这一层，后面的看板、编排、部署都会写两遍。

## User Intent Snapshot

```yaml
lake_or_ocean: 湖（海的第一片，前置依赖）
language: zh
source: user_intent
utterance: "现在想要的是支持桌面以及 web ui 的可视化，因为想要的底座后续是想要可以代替n8n加agent 的生产可部署稳定agent 项目"
goal: "传输层解耦：一套服务 + Wails 与 HTTP 两个适配器，同一份前端产物两种外壳可用"
non_goals: "工单看板 UI（L4）；工作流编排（L5）；容器与指标（L6）；拖拽编辑器（L7）"
constraints: "核心包不得 import wails；桌面端既有行为不得退化；HTTP 模式必须支持流式与审批"
scope_mode: focused
```

## Open Questions

| Q ID | Question | Recommendation | Status | Answer |
|------|----------|----------------|--------|--------|
| Q1 | 桌面与 web 是两套前端还是一套？ | 一套前端产物，传输层自动选择 | closed | 用户确认按推荐 1 执行 |
| Q2 | HTTP 事件用 SSE 还是 WebSocket？ | SSE，单向流足够，浏览器原生支持 | closed | 采用 SSE |

## Intent Coverage

| AC ID | Intent Source | Evidence |
|-------|---------------|----------|
| AC-401 | 目标: 桌面与 web 同时支持 | desktop/app.go 24 个方法直接挂在结构体上 |
| AC-402 | 目标: 生产可部署 | 全仓无 HTTP server |
| AC-403 | 目标: 一套界面 | 前端直接 import wailsjs 生成物 |
| AC-404 | 约束: 流式与审批 | 审批经 EventsEmit，浏览器拿不到 |
| AC-405 | 目标: 可部署稳定 | 无健康检查端点 |

## Acceptance Criteria

| AC ID | Criterion |
|-------|-----------|
| AC-401 | 提供与传输无关的服务层；该层不引用 wails 包；桌面与 HTTP 两个适配器用同一服务完成一次对话并收到相同的事件序列 |
| AC-402 | `agentdesk serve` 启动 HTTP 服务；`POST /api/chat` 以 SSE 下发 chat 事件与工具事件；`GET /api/sessions` 返回会话列表 |
| AC-403 | 前端只有一份构建产物；浏览器模式下通过 HTTP 传输，桌面模式下通过 Wails 传输，页面代码不区分 |
| AC-404 | HTTP 模式下审批请求经 SSE 下发；`POST /api/approvals/{id}` 提交决议后对话继续；拒绝时工具结果与本地模式一致 |
| AC-405 | `GET /healthz` 返回 200 与 JSON 状态；`GET /` 返回前端构建产物；未构建前端时返回明确错误而不是空白页 |

## 边界（本湖明确不做）

- 不做工单看板界面
- 不做工作流编排
- 不做容器与指标
- 不做鉴权与多用户
- 不改动既有 23 条 AC 的行为

## Parity Matrix

| Capability | Covered | Evidence |
|------------|---------|----------|
| 服务层解耦 | yes | AC-401 |
| HTTP + SSE | yes | AC-402 |
| 一套前端 | yes | AC-403 |
| 跨传输审批 | yes | AC-404 |
| 健康检查与静态托管 | yes | AC-405 |
