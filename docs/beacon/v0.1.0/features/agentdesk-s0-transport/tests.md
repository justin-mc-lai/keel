---
slug: agentdesk-s0-transport
version: v0.1.0
materials_status: current
---

# Tests: agentdesk-s0-transport

| AC ID | TC ID | Layer | Command | Assertion |
|-------|-------|-------|---------|-----------|
| AC-401 | TC-401 | behavior | `go test ./internal/core/ -run TestServiceIsTransportNeutral` | two_sinks_receive_the_same_event_sequence |
| AC-402 | TC-402 | behavior | `go test ./internal/httpserver/ -run TestChatStreamsOverSSE` | sse_carries_content_and_done |
| AC-403 | TC-403 | behavior | `go test ./internal/httpserver/ -run TestStaticServesFrontend` | index_served_and_missing_dist_reports_error |
| AC-404 | TC-404 | behavior | `go test ./internal/httpserver/ -run TestApprovalRoundTripOverHTTP` | approval_streamed_then_decision_resumes |
| AC-405 | TC-405 | behavior | `go test ./internal/httpserver/ -run TestHealthz` | health_returns_200_json |
