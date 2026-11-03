---
slug: agentdesk
version: v0.1.0
materials_status: current
---

# Evidence: agentdesk v0.1

All commands below were run on darwin/arm64, Go 1.25.6 (toolchain auto-upgraded to go1.26.10 by the wails v2 module), 2026-11-04.

## 1. Per-AC verification

| AC | Verification | Result |
|----|--------------|--------|
| AC-001 | `go test ./internal/provider/ -run TestRegistryListsProviders` | pass |
| AC-002 | `go test ./internal/provider/ -run TestStreamSplitsReasoningAndContent` | pass |
| AC-003 | `go test ./internal/agent/ -run TestLoopExecutesToolAndConverges` | pass |
| AC-004 | `go test ./desktop/ -run TestStreamEventsWired` + `wails build` | pass |
| AC-005 | `go test ./cmd/agentdesk/ -run TestRunCommandStreams` + real CLI run | pass |
| AC-006 | `go test ./internal/store/ -run TestSessionPersistsAcrossReopen` | pass |
| AC-007 | `go test ./internal/config/ -run TestSecretsNeverCommitted` | pass |
| AC-008 | `go test ./internal/tool/ -run TestUnregisteredToolRejected` | pass |
| AC-009 | `go test ./internal/smoke/ -run TestLiveProviders` (real network) | pass |
| AC-010 | `go test ./internal/config/ -run TestOpenSourceFilesPresent` | pass |

`beacon qa autotest run --run-id run_817baf058acf74624cc9e92d` → **10/10 passed**.

## 2. Real provider closure (the part that is not a mock)

```
$ ./bin/agentdesk providers
commandcode    ready      default=deepseek/deepseek-v4.1-flash
opencode-go    ready      default=deepseek-v4.1-flash

$ ./bin/agentdesk run -provider opencode-go -p "reply with exactly PONG"
PONG

$ ./bin/agentdesk run -provider commandcode -p "reply with exactly PONG"
PONG

$ ./bin/agentdesk smoke
opencode-go    OK   "PONG"  2816ms tokens=49
commandcode    OK   "PONG"  3267ms tokens=47
```

## 3. Real tool round trip (model → tool → model)

```
$ ./bin/agentdesk run -provider opencode-go -p "What time is it in Asia/Tokyo right now? Use the get_time tool."
I'll check that for you.
[tool] get_time
[tool result] 2026-11-04T02:10:08+09:00
The current time in Asia/Tokyo is **2026-11-04 02:10:08 (JST, UTC+9)**.
```

## 4. Desktop build

```
$ wails build
Built 'build/bin/agentdesk.app/Contents/MacOS/agentdesk' in 36.308s.

$ file build/bin/agentdesk.app/Contents/MacOS/agentdesk
Mach-O 64-bit executable arm64
$ ls -lh build/bin/agentdesk.app/Contents/MacOS/agentdesk
13M
```

The app was launched and rendered: sidebar with persisted sessions, provider dropdown
(`commandcode` / `opencode-go`), model dropdown, composer. Screenshot verified during the run.

## 5. Known external flakiness (recorded, not hidden)

`api.commandcode.ai` intermittently drops connections before the response headers from this
network. Measured 4/6 successes in one burst and 1/5 in another, in a period where
`opencode.ai` stayed 100%. The same autotest run failed TC-009 twice and then passed 10/10 twice
without any code change.

Mitigation in the code: `OpenAICompatible.Stream` retries the request up to 3 times on
transport-level errors before streaming begins (safe — nothing has been consumed yet), covered by
`TestStreamRetriesOnConnectionDrop`. Streaming itself is never replayed.

## 6. Reproduce

```bash
cp .env.example .env.local      # fill OPENCODE_GO_API_KEY / COMMAND_CODE_API_KEY
go test ./...
./bin/agentdesk smoke
$HOME/go/bin/wails build
```
