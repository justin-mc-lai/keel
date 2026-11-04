---
slug: agentdesk-l4-board
version: v0.1.0
materials_status: current
---

# Tests: agentdesk-l4-board

| AC ID | TC ID | Layer | Command | Assertion |
|-------|-------|-------|---------|-----------|
| AC-501 | TC-501 | behavior | `go test ./internal/httpserver/ -run TestTicketsAPI` | list_and_action_endpoints_work |
| AC-502 | TC-502 | behavior | `go test ./internal/core/ -run TestTicketsListAndHistory` | tickets_carry_state_and_history |
| AC-503 | TC-503 | behavior | `go test ./internal/core/ -run TestPanelSnapshotFromTickets` | needs_me_health_history_present |
| AC-504 | TC-504 | behavior | `go test ./internal/core/ -run TestTicketActionTransitions` | accept_reject_retry_change_state |
| AC-505 | TC-505 | behavior | `go test ./internal/httpserver/ -run TestTicketsEmptyAndError` | empty_list_is_200_and_bad_action_is_4xx |
