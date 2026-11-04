---
slug: agentdesk-l6-ops
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
  truth: docs/beacon/v0.1.0/features/agentdesk-l6-ops/truth.md
  tests: docs/beacon/v0.1.0/features/agentdesk-l6-ops/tests.md
  tasks: docs/beacon/v0.1.0/features/agentdesk-l6-ops/tasks.md
  evidence: docs/beacon/v0.1.0/features/agentdesk-l6-ops/evidence.md
---

# Requirement Truth: agentdesk-l6-ops

## 人话

流程能跑了，但它还得能被人运维：日志要能被机器读，指标要能被抓，数据要能备份与恢复，重启不能把正在跑的请求砍断。这一湖做这四件事。

## User Intent Snapshot

```yaml
lake_or_ocean: 湖
language: zh
source: user_intent
utterance: "想要的底座后续是想要可以代替n8n加agent 的生产可部署稳定agent 项目"
goal: "结构化日志、指标端点、备份与恢复、优雅停机"
non_goals: "容器镜像构建验收（本机 Docker daemon 未运行）；分布式追踪；告警；多副本"
constraints: "日志必须是可解析的 JSON；指标必须是 Prometheus 文本格式；停机必须在 10 秒内完成"
scope_mode: focused
```

## Open Questions

| Q ID | Question | Recommendation | Status | Answer |
|------|----------|----------------|--------|--------|
| Q1 | 容器镜像是否作为验收项？ | 只交付 Dockerfile，不纳入本湖验收 | closed | 本机 Docker daemon 未运行，构建无法验证，不写进 AC |
| Q2 | 指标用什么格式？ | Prometheus 文本格式，无依赖 | closed | 采用 |

## Intent Coverage

| AC ID | Intent Source | Evidence |
|-------|---------------|----------|
| AC-701 | 目标: 生产可运维 | 现状日志是普通文本 |
| AC-702 | 目标: 可观测 | 现状无指标端点 |
| AC-703 | 目标: 数据不丢 | 现状四类数据分散在四个文件，无备份手段 |
| AC-704 | 目标: 稳定 | 现状 serve 收到信号直接退出 |

## Acceptance Criteria

| AC ID | Criterion |
|-------|-----------|
| AC-701 | `serve -log-json` 输出 JSON 行日志，每行含 time、level、component、msg 四个字段 |
| AC-702 | `GET /metrics` 返回 Prometheus 文本格式，包含请求计数、对话轮次、token 总数、工单分状态计数四类指标 |
| AC-703 | `agentdesk backup <dir>` 生成带时间戳的备份目录并写清单；`agentdesk restore <dir>` 从备份恢复，恢复后文件内容与备份一致 |
| AC-704 | 收到 SIGTERM 时停止接受新请求，在 10 秒内关闭并退出码 0；正在处理的请求不被强制切断 |

## 边界（本湖明确不做）

- 不做镜像构建验收
- 不做分布式追踪
- 不做告警规则
- 不做多副本与一致性
- 不做自动定时备份

## Parity Matrix

| Capability | Covered | Evidence |
|------------|---------|----------|
| 结构化日志 | yes | AC-701 |
| 指标端点 | yes | AC-702 |
| 备份恢复 | yes | AC-703 |
| 优雅停机 | yes | AC-704 |
