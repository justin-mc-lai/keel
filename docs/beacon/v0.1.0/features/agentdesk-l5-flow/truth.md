---
slug: agentdesk-l5-flow
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
  truth: docs/beacon/v0.1.0/features/agentdesk-l5-flow/truth.md
  tests: docs/beacon/v0.1.0/features/agentdesk-l5-flow/tests.md
  tasks: docs/beacon/v0.1.0/features/agentdesk-l5-flow/tasks.md
  evidence: docs/beacon/v0.1.0/features/agentdesk-l5-flow/evidence.md
---

# Requirement Truth: agentdesk-l5-flow

## 人话

前面几湖把「一个会话、一张工单」做通了。这一湖做「一串步骤」：用 JSON 定义流程，引擎按依赖顺序跑，节点可重试、可超时，每次运行都留下记录。这是替代 n8n 的地基。

## User Intent Snapshot

```yaml
lake_or_ocean: 湖
language: zh
source: user_intent
utterance: "按照推荐1，进行"（配置定义 DAG + 可视化只读）
goal: "流程定义与校验、执行引擎（重试/退避/超时）、三种节点、运行记录持久化"
non_goals: "重放（L5b）；并行分支；可视化编辑器（L7）；节点市场"
constraints: "循环依赖与缺失依赖必须在执行前被拒；单节点失败不得静默跳过"
scope_mode: focused
```

## Open Questions

| Q ID | Question | Recommendation | Status | Answer |
|------|----------|----------------|--------|--------|
| Q1 | 流程文件用 YAML 还是 JSON？ | JSON，标准库可直接解析 | closed | 采用 JSON |
| Q2 | 首版支持哪些节点？ | http、agent、delay 三种 | closed | 采用三种 |
| Q3 | 重放是否本湖交付？ | 延后到 L5b，先保证执行与记录 | closed | 本湖不做重放 |

## Intent Coverage

| AC ID | Intent Source | Evidence |
|-------|---------------|----------|
| AC-601 | 目标: 配置定义 DAG | 现无任何流程概念 |
| AC-602 | 目标: 生产可用 | 现无重试、退避、超时 |
| AC-603 | 目标: 覆盖常见步骤 | 现只有单轮对话 |
| AC-604 | 目标: 可审计 | 现无运行记录 |

## Acceptance Criteria

| AC ID | Criterion |
|-------|-----------|
| AC-601 | 从 JSON 载入流程；节点 id 重复、依赖不存在、存在环三种情况都在执行前返回错误 |
| AC-602 | 引擎按拓扑序执行；节点支持 attempts 重试与指数退避；节点支持 timeout，超时按失败处理 |
| AC-603 | 支持 http、agent、delay 三种节点；节点输出可通过 `{{节点id}}` 注入后续节点的 prompt 与 url |
| AC-604 | 每次运行写一条记录（含整体状态、每节点状态、尝试次数、耗时、输出内容与错误信息），并可读回 |

## 边界（本湖明确不做）

- 不做并行分支与条件分支
- 不做运行重放
- 不做可视化编辑器
- 不做跨机执行
- 不做流程版本迁移

## Parity Matrix

| Capability | Covered | Evidence |
|------------|---------|----------|
| 定义与校验 | yes | AC-601 |
| 执行与容错 | yes | AC-602 |
| 三种节点 | yes | AC-603 |
| 运行记录 | yes | AC-604 |
