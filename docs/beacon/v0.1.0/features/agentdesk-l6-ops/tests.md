---
slug: agentdesk-l6-ops
version: v0.1.0
materials_status: current
---

# Tests: agentdesk-l6-ops

| AC ID | TC ID | Layer | Command | Assertion |
|-------|-------|-------|---------|-----------|
| AC-701 | TC-701 | behavior | `go test ./internal/obs/ -run TestJSONLogHasFourFields` | json_log_line_carries_time_level_component_msg |
| AC-702 | TC-702 | behavior | `go test ./internal/obs/ -run TestMetricsRenderPrometheus` | counters_and_gauges_render_in_prometheus_text |
| AC-703 | TC-703 | behavior | `go test ./internal/backup/ -run TestBackupRestoreRoundTrip` | backup_then_restore_restores_identical_files |
| AC-704 | TC-704 | behavior | `go test ./internal/appserver/ -run TestGracefulShutdown` | cancel_stops_server_within_deadline |
