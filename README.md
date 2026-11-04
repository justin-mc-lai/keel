# Keel

> 定位与演进见 [docs/vision/agent-base-positioning.md](docs/vision/agent-base-positioning.md)。
> 旧名 AgentDesk，2026-11 更名为 Keel。

A desktop LLM agent base in Go: **one binary, no Node runtime, no Python, no local HTTP server.**

Provider-agnostic by construction — swapping from one vendor to another is a config value, not a code change. Tokens travel through Wails' in-process event channel, so streaming works without SSE and without binding a port.

```
Go + Wails v2 + Vue 3  →  single executable
        │
        ├── provider/   OpenAI-compatible streaming, one interface
        ├── agent/      tool-calling loop, bounded iterations
        ├── tool/       registry + JSON Schema + approval gate
        └── store/      SQLite (pure Go, CGO-free)
```

## Why another one

The Go LLM ecosystem has plenty of provider abstractions and server-side agent frameworks. What it does not have is an MIT-licensed **desktop** base that is provider-agnostic, streams in-process, and builds into one file. That is the gap this project fills.

Non-goals for v0.1: multi-agent orchestration, MCP client, image/voice, vendor-native wires (OpenAI Responses / Anthropic Messages).

## Quick start

```bash
git clone https://github.com/justin-mc-lai/keel
cd keel

cp .env.example .env.local     # then fill in one provider key
go build -o keel ./cmd/keel

./keel providers
./keel run -p "reply with exactly PONG"
./keel smoke
```

Desktop app:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails build
open build/bin/keel.app
```

## Providers

Two presets ship with the binary. Keys come from the environment (or a gitignored `.env.local`), never from the repo.

| Preset | Endpoint | Key variable | Notes |
|---|---|---|---|
| `opencode-go` | `https://opencode.ai/zen/go/v1` | `OPENCODE_GO_API_KEY` | sends `x-opencode-session` per conversation |
| `commandcode` | `https://api.commandcode.ai/provider/v1` | `COMMAND_CODE_API_KEY` | forces a direct connection; long streams break behind a local proxy |

Add or override providers in `~/.keel/config.json` (or the path in `KEEL_CONFIG`):

```json
{
  "defaultProvider": "opencode-go",
  "providers": [
    {
      "name": "my-gateway",
      "baseURL": "https://example.com/v1",
      "apiKey": "${MY_GATEWAY_KEY}",
      "defaultModel": "some-model",
      "models": ["some-model"],
      "noProxy": false
    }
  ]
}
```

`${VAR}` in `apiKey` is resolved from the environment, so the config file never has to hold a secret.

## Two shells, one build

The same frontend build runs in both places:

```bash
wails build                    # desktop window
keel serve                # browser at http://127.0.0.1:8787
keel serve -addr :9000 -assets frontend/dist
```

`internal/core` holds every piece of logic and imports no UI package. The Wails
adapter pushes events through `runtime.EventsEmit`; the HTTP adapter pushes the
same events as Server-Sent Events. `frontend/src/transport.ts` picks the right
one at runtime, so `App.vue` has a single code path.

HTTP surface:

| Method | Path | Purpose |
|---|---|---|
| GET | `/healthz` | liveness JSON |
| GET | `/api/providers` | configured vendors |
| GET/POST | `/api/sessions` | list / create |
| PATCH/DELETE | `/api/sessions/{id}` | rename / delete |
| GET | `/api/sessions/{id}/messages` | history |
| GET | `/api/sessions/{id}/export` | JSON export |
| POST | `/api/chat` | SSE stream of chat and tool events |
| POST | `/api/approvals/{id}` | answer a pending approval |
| GET/POST | `/api/permission` | read / set the tier |
| GET/POST | `/api/settings` | read / save settings |
| GET | `/api/usage` | token rollup |

## Workspace and tools

Every session is bound to a workspace directory. File tools resolve every path
through that root; a path that escapes it (via `..`, an absolute path, or a
symlink) is refused before any I/O happens.

Nine built-in tools:

| Tool | Approval | Notes |
|---|---|---|
| `read_file` | no | 256 KB cap |
| `write_file` | yes | creates parent directories |
| `edit_file` | yes | refuses unless the needle appears exactly once |
| `list_dir` | no | directories end with `/` |
| `grep` | no | regex, skips `.git` and `node_modules`, 1 MB file cap |
| `fetch_url` | no | http/https only, 100 KB cap, 20s timeout |
| `run_shell` | yes | runs in the workspace root, 30s, output capped at 8 KB |
| `get_time` | no | IANA timezone |
| `echo` | no | round-trip check |

`@relative/path` in a message is expanded to that file's contents before the
turn starts. A reference that does not resolve inside the workspace is left
alone, so an email address never breaks a message.

`AGENTS.md` at the workspace root is appended to the system prompt for every
turn.

Set the root with `keel run -w /path` or in the desktop settings panel.

## Long commands, diffs, cost

A shell command that takes longer than 5 seconds is detached into a background
job and returns an id immediately; `job_output` reads it, `job_kill` stops it.
Jobs have a 30-minute lifetime and their process group dies with them.

`write_file` and `edit_file` emit a unified diff before touching disk, so the
UI can show what is about to change.

Every call's token usage is appended to `~/.keel/usage.jsonl`.
`keel stats` prints totals per provider and per session.

## Running it

```bash
keel serve -addr 0.0.0.0:8787 -log-json
```

| Endpoint | Purpose |
|---|---|
| `/healthz` | liveness JSON |
| `/metrics` | Prometheus text: requests, tickets per state |

Logs are JSON lines (`time`, `level`, `component`, `msg`) when `-log-json` is
set. SIGTERM stops accepting new requests, drains for up to 10 seconds, logs
`stopped`, and exits 0.

State lives in `KEEL_STATE_DIR` (default `~/.keel`): the session
database, tickets, runs, usage and channel files.

```bash
keel backup /backups     # timestamped directory + manifest
keel restore /backups/keel-20261105-000053
```

A `Dockerfile` is included (multi-stage, distroless, non-root, CGO off). Its
image build is **not verified** — the Docker daemon was not running on the
machine that produced this release.

## Workflows

A workflow is a JSON DAG. Nodes run in dependency order; a failing node stops
the run.

```json
{
  "id": "hello-flow",
  "nodes": [
    { "id": "fetch", "type": "http",  "url": "https://example.com",
      "timeout": "15s", "retry": { "attempts": 2, "backoff": "300ms" } },
    { "id": "think", "type": "agent", "depends": ["fetch"],
      "prompt": "In one sentence, say what this page is: {{fetch}}" },
    { "id": "wait",  "type": "delay", "depends": ["think"], "duration": "10ms" }
  ]
}
```

```bash
keel workflow run flow.json -provider opencode-go
keel runs
```

Node types: `http`, `agent`, `delay`. `{{node-id}}` substitutes a previous
node's output. Every run is appended to `~/.keel/runs.json` with per-node
status, attempts, timing and error text.

Validation happens before anything executes: duplicate ids, unknown
dependencies and cycles are all refused.

Out of scope for this lake: parallel branches, conditional branches, replay.

## Board

The sidebar switches between **对话** and **工单**. The board groups tickets by
state, shows the three panel questions as a summary row, and lets you accept,
reject or retry straight from a card. Clicking a card shows its full history.

Everything comes from the same core service, so the desktop window and the
browser show the same board.

## Triggers, tickets, panel

This is the part that makes it a base rather than a chat client.

A **trigger** is an immutable event with a dedup key. A **schedule** produces
them on a plan; `@every 30s` and `daily 09:00` are supported. The ledger is
append-only, so a restart never replays an instant.

A **ticket** is one authorized piece of work with its own state machine:

```
open -> working -> awaiting_review -> ended
  |         |            |
  |         +-> awaiting_person
  +-> queued -> working
working -> failed -> queued
```

Illegal moves are refused and leave the ticket untouched. **Slots** cap how many
tickets run at once; the rest queue and start automatically when a slot frees.

Finished tickets write one record through a **Channel** (local file by default).

`keel panel` answers three questions: what needs me, is the system healthy,
and what happened to one ticket.

```bash
keel tickets
keel panel
keel panel -ticket <id>
keel panel -json
```

## MCP servers

stdio servers are configured in `~/.keel/config.json`:

```json
{
  "mcpServers": {
    "files": { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"] }
  }
}
```

At startup each server is spawned, `initialize` is sent, and `tools/list` is
registered as `mcp_<server>_<tool>`. A server that fails to start is logged and
skipped; the remaining servers and built-in tools stay available.

## Permissions

Tool calls are gated. Three tiers, switchable in the toolbar and persisted per session:

| Tier | Behaviour |
|---|---|
| `ask` (default) | every gated call shows an approval card; 120s with no answer = denied |
| `allow` | asks once per tool per session, then remembers |
| `bypass` | never asks, and the toolbar shows a persistent red warning |

`run_shell` is the one built-in tool that requires approval, so the tiers have something real to gate. Approved calls run with a 30s deadline, kill their whole process group on timeout, and cap output at 8 KB.

The approval window is bounded on purpose: the call blocks the agent loop, so an unanswered card must not park a turn forever. Cancelling the turn also cancels the pending approval.

## CLI

```bash
keel run -p "…" [-provider name] [-model id] [-effort level] [-session id] [-yes]
keel providers
keel sessions
keel history <session-id>
keel smoke
```

`run` streams tokens to stdout, persists the conversation to SQLite, and prints the session id on stderr so it stays out of piped output. `-yes` auto-approves gated tools; without it a gated call is denied and the model is told so. `-effort` maps to `reasoning_effort` (off / low / medium / high) and is omitted when unset.

## Architecture notes

**The provider interface is the product.** Everything else — loop, tools, store, CLI, desktop — depends only on:

```go
type Provider interface {
    Name() string
    DefaultModel() string
    Models() []string
    Stream(ctx context.Context, req Request) (<-chan Event, error)
}
```

**Reasoning is not content.** Vendors that emit `reasoning_content` produce a different event type, so the UI can collapse thinking instead of mixing it into the answer.

**Desktop transport is a channel, not HTTP.** `runtime.EventsEmit` forwards `provider.Event` straight to Vue. The channel shape is retained so a headless SSE adapter can be added later without touching the agent loop.

**Tools are refused by default.** A call to an unregistered tool returns `ErrUnknownTool`; a tool with `RequiresApproval` returns `ErrDenied` unless an approver says yes.

## Development

```bash
go test ./...                        # unit tests
go test ./internal/smoke -run TestLiveProviders -v   # real API calls (needs keys)
cd frontend && npm install && npm run build
wails build
```

## License

MIT — see [LICENSE](LICENSE).
