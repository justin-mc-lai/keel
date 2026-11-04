# AgentDesk

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
git clone https://github.com/justin-mc-lai/agentdesk
cd agentdesk

cp .env.example .env.local     # then fill in one provider key
go build -o agentdesk ./cmd/agentdesk

./agentdesk providers
./agentdesk run -p "reply with exactly PONG"
./agentdesk smoke
```

Desktop app:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails build
open build/bin/agentdesk.app
```

## Providers

Two presets ship with the binary. Keys come from the environment (or a gitignored `.env.local`), never from the repo.

| Preset | Endpoint | Key variable | Notes |
|---|---|---|---|
| `opencode-go` | `https://opencode.ai/zen/go/v1` | `OPENCODE_GO_API_KEY` | sends `x-opencode-session` per conversation |
| `commandcode` | `https://api.commandcode.ai/provider/v1` | `COMMAND_CODE_API_KEY` | forces a direct connection; long streams break behind a local proxy |

Add or override providers in `~/.agentdesk/config.json` (or the path in `AGENTDESK_CONFIG`):

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

Set the root with `agentdesk run -w /path` or in the desktop settings panel.

## Long commands, diffs, cost

A shell command that takes longer than 5 seconds is detached into a background
job and returns an id immediately; `job_output` reads it, `job_kill` stops it.
Jobs have a 30-minute lifetime and their process group dies with them.

`write_file` and `edit_file` emit a unified diff before touching disk, so the
UI can show what is about to change.

Every call's token usage is appended to `~/.agentdesk/usage.jsonl`.
`agentdesk stats` prints totals per provider and per session.

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
agentdesk run -p "…" [-provider name] [-model id] [-effort level] [-session id] [-yes]
agentdesk providers
agentdesk sessions
agentdesk history <session-id>
agentdesk smoke
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
