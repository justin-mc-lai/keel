---
slug: agentdesk-l3-base
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
  truth: docs/beacon/v0.1.0/features/agentdesk-l3-base/truth.md
  tests: docs/beacon/v0.1.0/features/agentdesk-l3-base/tests.md
  tasks: docs/beacon/v0.1.0/features/agentdesk-l3-base/tasks.md
  evidence: docs/beacon/v0.1.0/features/agentdesk-l3-base/evidence.md
---

# Requirement Truth: agentdesk-l3-base

## 人话

L1 让它能用，L2 让它能干活。L3 让它能接活：外部事件进来变成工单，定时任务到点自己派活，工单有状态机与看板，执行有并发槽位，结束能回写。这是从「聊天客户端」变成「Agent 底座」的那一层。

## User Intent Snapshot

```yaml
lake_or_ocean: 湖（三湖计划中的第三湖）
language: zh
source: user_intent
utterance: "l3除了以上，还需要有定时任务触发"
goal: "L3 底座：定时触发、Trigger 幂等、Ticket 状态机、执行槽位、订阅回写、面板三问"
non_goals: "真实 GitHub/飞书/邮件账号接入（只留适配接口与本地实现）；跨机执行；多租户"
constraints: "定时重启后不得重复触发同一时刻；同 dedup_key 只产生一张工单；并发上限必须可配"
scope_mode: focused
```

## Open Questions

| Q ID | Question | Recommendation | Status | Answer |
|------|----------|----------------|--------|--------|
| Q1 | 定时计划要支持完整 cron 吗？ | 先支持间隔与每日定时刻两种，完整 cron 语法延后 | closed | 采用 `@every` 与 `daily HH:MM`，避免引入 cron 解析依赖 |
| Q2 | 外部账号接入是否本湖必做？ | 只做接口与本地实现，真实账号下一版 | closed | 采用 Channel 接口，本地文件回写作为默认实现 |

## Intent Coverage

| AC ID | Intent Source | Evidence |
|-------|---------------|----------|
| AC-301 | utterance: 还需要有定时任务触发 | 现状无任何调度能力 |
| AC-302 | 参考 dsh Trigger 不可变与幂等 | 现状无事件概念 |
| AC-303 | 参考 dsh Ticket 自持状态 | 现状只有会话，没有工单 |
| AC-304 | 参考 dsh worker slot | 现状单会话串行 |
| AC-305 | 参考 dsh Channel 回写 | 现状结果只留在会话里 |
| AC-306 | 参考 dsh 面板三问 | 现状只有会话列表 |

## Acceptance Criteria

| AC ID | Criterion |
|-------|-----------|
| AC-301 | 支持 `@every <duration>` 与 `daily HH:MM` 两种计划；到点产出 Trigger；进程重启后同一计划同一时刻不重复触发 |
| AC-302 | 每个 Trigger 带 dedup_key；相同 dedup_key 只产生一张 Ticket，重复入库被拒绝 |
| AC-303 | Ticket 有 open、working、awaiting_person、awaiting_review、ended 五个状态；非法转移返回错误且状态不变 |
| AC-304 | 执行槽位有可配置并发上限；超过上限的工单进入 queued；槽位释放后自动取下一个 queued 工单 |
| AC-305 | 提供 Channel 接口与本地文件实现；Ticket 结束时写一条回写记录，记录含工单 id、结果与时间 |
| AC-306 | `agentdesk panel` 输出三问：需要我处理的工单、系统健康（各状态计数与槽位占用）、单个工单从 Trigger 到结束的历史 |

## FSM 五元组

| State | Event | Guard | To | Action |
|-------|-------|-------|----|--------|
| open | start | 槽位可用 | working | 绑定执行器 |
| open | queue | 槽位已满 | queued | 入队 |
| queued | start | 槽位释放 | working | 出队并绑定 |
| working | ask | 需要人类输入 | awaiting_person | 记录问题 |
| awaiting_person | answer | 收到答复 | working | 继续执行 |
| working | submit | 产出结果 | awaiting_review | 记录结果 |
| awaiting_review | accept | 人工接受 | ended | 回写 Channel |
| awaiting_review | reject | 人工拒绝 | working | 记录原因 |
| working | fail | 执行失败 | failed | 记录原因 |
| failed | retry | 人工重试 | queued | 重新入队 |
| awaiting_person | stop | 人工停止 | ended | 回写 Channel |

终态：ended

- illegal: ended -> working
- illegal: ended -> open
- illegal: open -> awaiting_review
- illegal: queued -> awaiting_person
- illegal: awaiting_review -> queued

## 边界（本湖明确不做）

- 不做真实 GitHub / 飞书 / 邮件账号接入
- 不做跨机执行与远程执行器
- 不做多租户与权限分离
- 不做完整 cron 语法（`*`、范围、列表）
- 不做桌面看板 UI，本湖只做 CLI 面板

## Parity Matrix

| Capability | Covered | Evidence |
|------------|---------|----------|
| 定时触发 | yes | AC-301 |
| Trigger 幂等 | yes | AC-302 |
| 工单状态机 | yes | AC-303 |
| 执行槽位 | yes | AC-304 |
| 订阅回写 | yes | AC-305 |
| 面板三问 | yes | AC-306 |
