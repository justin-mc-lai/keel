---
slug: agentdesk-l5-flow
version: v0.1.0
materials_status: current
---

# Tests: agentdesk-l5-flow

| AC ID | TC ID | Layer | Command | Assertion |
|-------|-------|-------|---------|-----------|
| AC-601 | TC-601 | behavior | `go test ./internal/workflow/ -run TestDefinitionRejectsBadGraphs` | duplicate_missing_and_cycle_refused |
| AC-602 | TC-602 | behavior | `go test ./internal/workflow/ -run TestEngineRetriesAndTimesOut` | retry_then_timeout_is_failure |
| AC-603 | TC-603 | behavior | `go test ./internal/workflow/ -run TestEngineNodesAndTemplates` | http_agent_delay_and_substitution |
| AC-604 | TC-604 | behavior | `go test ./internal/workflow/ -run TestRunRecordPersisted` | run_and_node_status_round_trip |
