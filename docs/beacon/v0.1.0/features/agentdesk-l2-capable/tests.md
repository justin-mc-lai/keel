---
slug: agentdesk-l2-capable
version: v0.1.0
materials_status: current
---

# Tests: agentdesk-l2-capable

| AC ID | TC ID | Layer | Command | Assertion |
|-------|-------|-------|---------|-----------|
| AC-201 | TC-201 | behavior | `go test ./internal/jobs/ -run TestLongCommandDetachesAndReturns` | detaches_after_threshold_and_output_is_readable |
| AC-202 | TC-202 | behavior | `go test ./internal/tool/ -run TestDiffPreviewBeforeWrite` | diff_emitted_before_write_and_denial_keeps_file |
| AC-203 | TC-203 | behavior | `go test ./internal/usage/ -run TestUsageRollup` | totals_group_by_provider_and_session |
| AC-204 | TC-204 | behavior | `go test ./internal/mcp/ -run TestStdioServerRegistersTools` | tools_listed_and_callable_over_stdio |
| AC-205 | TC-205 | behavior | `go test ./internal/agent/ -run TestSubagentIsolatedSession` | child_history_not_visible_to_parent |
