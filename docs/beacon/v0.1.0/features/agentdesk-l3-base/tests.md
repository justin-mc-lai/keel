---
slug: agentdesk-l3-base
version: v0.1.0
materials_status: current
---

# Tests: agentdesk-l3-base

| AC ID | TC ID | Layer | Command | Assertion |
|-------|-------|---------|-----------|-----------|
| AC-301 | TC-301 | behavior | `go test ./internal/trigger/ -run TestScheduleFiresOncePerInstant` | same_schedule_instant_fires_once_across_restart |
| AC-302 | TC-302 | behavior | `go test ./internal/trigger/ -run TestDedupKeyIsIdempotent` | duplicate_dedup_key_yields_one_ticket |
| AC-303 | TC-303 | behavior | `go test ./internal/ticket/ -run TestFSMRejectsIllegalTransitions` | five_states_and_illegal_moves_refused |
| AC-304 | TC-304 | behavior | `go test ./internal/ticket/ -run TestSlotsRespectLimitAndDrain` | queue_when_full_and_start_next_on_release |
| AC-305 | TC-305 | behavior | `go test ./internal/channel/ -run TestLocalChannelWritesOnEnd` | ended_ticket_writes_one_record |
| AC-306 | TC-306 | behavior | `go test ./internal/panel/ -run TestPanelAnswersThreeQuestions` | needs_me_health_and_history_present |
