---
slug: agentdesk-l1-usable
version: v0.1.0
materials_status: current
---

# Tests: agentdesk-l1-usable

| AC ID | TC ID | Layer | Command | Assertion |
|-------|-------|-------|---------|-----------|
| AC-101 | TC-101 | behavior | `go test ./internal/workspace/ -run TestSandboxRejectsEscape` | absolute_and_parent_escape_are_refused |
| AC-102 | TC-102 | behavior | `go test ./internal/tool/ -run TestFileToolsRoundTrip` | six_tools_declared_and_write_read_edit_work |
| AC-103 | TC-103 | behavior | `go test ./internal/agent/ -run TestAttachmentExpansion` | at_path_expanded_into_user_message |
| AC-104 | TC-104 | behavior | `go test ./internal/store/ -run TestSessionManage` | rename_delete_search_export_round_trip |
| AC-105 | TC-105 | behavior | `go test ./internal/config/ -run TestSettingsPersist` | saved_settings_reload_identical |
| AC-106 | TC-106 | behavior | `go test ./internal/workspace/ -run TestAgentsMdLoaded` | agents_md_text_returned_when_present |
| AC-107 | TC-107 | behavior | `go test ./internal/store/ -run TestRetryTruncates` | messages_after_last_user_turn_are_removed |
