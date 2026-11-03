---
slug: agentdesk-permissions
version: v0.1.0
materials_status: current
---

# Tests: agentdesk-permissions

| AC ID | TC ID | Layer | Command | Assertion |
|-------|-------|-------|---------|-----------|
| AC-011 | TC-011 | behavior | `go test ./internal/perm/ -run TestModeDefaultAndSessionAllow` | ask_is_default_and_allow_is_per_tool_per_session |
| AC-012 | TC-012 | behavior | `go test ./internal/agent/ -run TestApprovalBlocksThenResumes` | approval_request_emitted_then_turn_resumes |
| AC-013 | TC-013 | behavior | `go test ./internal/agent/ -run TestApprovalTimeoutAndCancel` | timeout_denies_and_cancel_unblocks |
| AC-014 | TC-014 | behavior | `go test ./desktop/ -run TestBypassWarningVisible` | bypass_skips_approval_and_sets_warning |
| AC-015 | TC-015 | behavior | `go test ./internal/provider/ -run TestReasoningEffortForwarded` | effort_written_when_set_and_omitted_when_empty |
