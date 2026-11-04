---
slug: agentdesk-l3-base
version: v0.1.0
materials_status: current
task_source: acceptance_criteria
---

# Tasks: agentdesk-l3-base

## Task Ledger

- [x] TASK-301 定时计划与 Trigger 产出（AC-301）· ac=AC-301· evidence=TestScheduleFiresOncePerInstant
- [x] TASK-302 dedup_key 幂等（AC-302）· ac=AC-302· evidence=TestDedupKeyIsIdempotent
- [x] TASK-303 Ticket 五状态 FSM（AC-303）· ac=AC-303· evidence=TestFSMRejectsIllegalTransitions
- [x] TASK-304 执行槽位与排队（AC-304）· ac=AC-304· evidence=TestSlotsRespectLimitAndDrain
- [x] TASK-305 Channel 回写（AC-305）· ac=AC-305· evidence=TestLocalChannelWritesOnEnd
- [x] TASK-306 面板三问（AC-306）· ac=AC-306· evidence=TestPanelAnswersThreeQuestions

## Boundary

Checkboxes are AC-bound ledgers. `[x]` is valid only when the bound evidence exists (completion ≠ self-tick).
