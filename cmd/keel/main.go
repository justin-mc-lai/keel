// Command keel is the headless entry point. The desktop app and this CLI
// share the same provider, tool, store and agent packages; only the transport
// differs (stdout here, Wails events in the app).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/justin-mc-lai/keel/internal/agent"
	"github.com/justin-mc-lai/keel/internal/appserver"
	"github.com/justin-mc-lai/keel/internal/backup"
	"github.com/justin-mc-lai/keel/internal/config"
	"github.com/justin-mc-lai/keel/internal/core"
	"github.com/justin-mc-lai/keel/internal/httpserver"
	"github.com/justin-mc-lai/keel/internal/mcp"
	"github.com/justin-mc-lai/keel/internal/obs"
	"github.com/justin-mc-lai/keel/internal/panel"
	"github.com/justin-mc-lai/keel/internal/provider"
	"github.com/justin-mc-lai/keel/internal/store"
	"github.com/justin-mc-lai/keel/internal/ticket"
	"github.com/justin-mc-lai/keel/internal/tool"
	usageledger "github.com/justin-mc-lai/keel/internal/usage"
	"github.com/justin-mc-lai/keel/internal/workflow"
	"github.com/justin-mc-lai/keel/internal/workspace"
	fsimpl "io/fs"
)

func main() {
	os.Exit(Execute(os.Args[1:], os.Stdout, os.Stderr))
}

// Execute runs one CLI invocation and returns the process exit code.
func Execute(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		usage(errOut)
		return 2
	}
	switch args[0] {
	case "run":
		return cmdRun(args[1:], out, errOut)
	case "providers":
		return cmdProviders(out, errOut)
	case "sessions":
		return cmdSessions(out, errOut)
	case "history":
		return cmdHistory(args[1:], out, errOut)
	case "smoke":
		return cmdSmoke(out, errOut)
	case "stats":
		return cmdStats(out, errOut)
	case "panel":
		return cmdPanel(args[1:], out, errOut)
	case "serve":
		return cmdServe(args[1:], out, errOut)
	case "workflow":
		return cmdWorkflow(args[1:], out, errOut)
	case "runs":
		return cmdRuns(out, errOut)
	case "healthcheck":
		return cmdHealthcheck(out, errOut)
	case "backup":
		return cmdBackup(args[1:], out, errOut)
	case "restore":
		return cmdRestore(args[1:], out, errOut)
	case "tickets":
		return cmdTickets(out, errOut)
	case "version", "-v", "--version":
		fmt.Fprintln(out, "keel "+Version)
		return 0
	case "help", "-h", "--help":
		usage(out)
		return 0
	default:
		fmt.Fprintf(errOut, "unknown command %q\n\n", args[0])
		usage(errOut)
		return 2
	}
}

// Version is the release version.
const Version = "0.1.0"

func usage(w io.Writer) {
	fmt.Fprint(w, `keel — a provider-agnostic desktop LLM agent base

usage:
  keel run -p "prompt" [-provider name] [-model id] [-effort level] [-session id] [-yes]
  keel providers
  keel sessions
  keel history <session-id>
  keel smoke
  keel stats
  keel tickets
  keel panel [-ticket id] [-json]
  keel serve [-addr 127.0.0.1:8787] [-assets frontend/dist]
  keel workflow run <file.json> [-provider name] [-model id]
  keel runs
  keel healthcheck
  keel backup <dir>
  keel restore <backup-dir>
  keel version
`)
}

func cmdRun(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(errOut)
	prompt := fs.String("p", "", "prompt text (required)")
	providerName := fs.String("provider", "", "provider name (default: config default)")
	model := fs.String("model", "", "model id (default: provider default)")
	session := fs.String("session", "", "session id to continue (default: new session)")
	approve := fs.Bool("yes", false, "auto-approve tools that require approval")
	effort := fs.String("effort", "", "reasoning effort: off, low, medium, high")
	wsRoot := fs.String("w", "", "workspace root (default: current directory)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*prompt) == "" {
		fmt.Fprintln(errOut, "run: -p is required")
		return 2
	}

	reg, err := config.Load()
	if err != nil {
		fmt.Fprintf(errOut, "run: %v\n", err)
		return 1
	}
	p, err := pick(reg, *providerName)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	if reg.DefaultName() == "" {
		fmt.Fprintln(errOut, "run: no provider configured; set OPENCODE_GO_API_KEY or COMMAND_CODE_API_KEY")
		return 1
	}
	st, err := store.Open("")
	if err != nil {
		fmt.Fprintf(errOut, "run: %v\n", err)
		return 1
	}
	defer st.Close()

	sid := *session
	if sid == "" {
		sid, err = st.CreateSession("")
		if err != nil {
			fmt.Fprintf(errOut, "run: %v\n", err)
			return 1
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	answer, err := runPrompt(ctx, p, st, sid, *prompt, *model, *effort, *wsRoot, *approve, out)
	if err != nil {
		fmt.Fprintf(errOut, "\nrun: %v\n", err)
		return 1
	}
	if !strings.HasSuffix(answer, "\n") {
		fmt.Fprintln(out)
	}
	fmt.Fprintf(errOut, "[session %s]\n", sid)
	return 0
}

// runPrompt is the testable core of `keel run`.
func runPrompt(
	ctx context.Context,
	p provider.Provider,
	st *store.Store,
	sessionID, prompt, model, effort, wsRoot string,
	autoApprove bool,
	out io.Writer,
) (string, error) {
	ws, err := workspace.Open(wsRoot)
	if err != nil {
		return "", err
	}
	tools, err := tool.NewDefaultRegistry(func(ctx context.Context, name string, args []byte) bool { return autoApprove }, ws)
	if err != nil {
		return "", err
	}
	if cfg, err := config.LoadFile(); err == nil && len(cfg.MCPServers) > 0 {
		clients, errs := mcp.StartAll(cfg.MCPServers)
		for _, e := range errs {
			fmt.Fprintf(out, "[mcp] %v\n", e)
		}
		for _, e := range mcp.RegisterTools(tools, clients) {
			fmt.Fprintf(out, "[mcp] %v\n", e)
		}
	}

	loop := agent.New(agent.Options{
		Provider:     p,
		Tools:        tools,
		Store:        st,
		Model:        model,
		Effort:       effort,
		Instructions: ws.Instructions(),
		OnEvent: func(ev provider.Event) {
			if ev.Type == provider.EventUsage && ev.Usage != nil {
				recordUsage(p.Name(), model, sessionID, ev.Usage)
			}
			switch ev.Type {
			case provider.EventContent:
				fmt.Fprint(out, ev.Text)
			case provider.EventReasoning:
				// Reasoning is intentionally not mixed into stdout: it would
				// corrupt piped output. The desktop UI shows it in its own pane.
			case provider.EventToolStart:
				fmt.Fprintf(out, "\n[tool] %s\n", ev.ToolCall.Name)
			case provider.EventToolDone:
				fmt.Fprintf(out, "[tool result] %s\n", oneLine(ev.Text, 200))
			}
		},
	})
	if err := tools.SetSubagent(loop.Subagent()); err != nil {
		return "", err
	}
	return loop.Run(ctx, sessionID, prompt)
}

func cmdProviders(out, errOut io.Writer) int {
	reg, err := config.Load()
	if err != nil {
		fmt.Fprintf(errOut, "providers: %v\n", err)
		return 1
	}
	for _, info := range reg.List() {
		state := "no key"
		if info.Available {
			state = "ready"
		}
		fmt.Fprintf(out, "%-14s %-10s default=%s\n", info.Name, state, info.DefaultModel)
		for _, m := range info.Models {
			fmt.Fprintf(out, "  - %s\n", m)
		}
	}
	return 0
}

func cmdSessions(out, errOut io.Writer) int {
	st, err := store.Open("")
	if err != nil {
		fmt.Fprintf(errOut, "sessions: %v\n", err)
		return 1
	}
	defer st.Close()
	list, err := st.ListSessions()
	if err != nil {
		fmt.Fprintf(errOut, "sessions: %v\n", err)
		return 1
	}
	for _, s := range list {
		fmt.Fprintf(out, "%s  %s  %s\n", s.ID, time.Unix(s.UpdatedAt, 0).Format("2006-01-02 15:04"), s.Title)
	}
	return 0
}

func cmdHistory(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "history: session id required")
		return 2
	}
	st, err := store.Open("")
	if err != nil {
		fmt.Fprintf(errOut, "history: %v\n", err)
		return 1
	}
	defer st.Close()
	msgs, err := st.Messages(args[0])
	if err != nil {
		fmt.Fprintf(errOut, "history: %v\n", err)
		return 1
	}
	for _, m := range msgs {
		body := m.Content
		if body == "" && len(m.ToolCalls) > 0 {
			var parts []string
			for _, c := range m.ToolCalls {
				parts = append(parts, c.Name+"("+c.Arguments+")")
			}
			body = "→ " + strings.Join(parts, ", ")
		}
		fmt.Fprintf(out, "[%s] %s\n", m.Role, body)
	}
	return 0
}

func cmdSmoke(out, errOut io.Writer) int {
	reg, err := config.Load()
	if err != nil {
		fmt.Fprintf(errOut, "smoke: %v\n", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	available := map[string]bool{}
	for _, info := range reg.List() {
		available[info.Name] = info.Available
	}

	failed := 0
	for _, name := range reg.Names() {
		p, _ := reg.Get(name)
		if !available[name] {
			fmt.Fprintf(out, "%-14s SKIP (no key)\n", name)
			continue
		}
		start := time.Now()
		var text strings.Builder
		stream, err := p.Stream(ctx, provider.Request{
			Messages:  []provider.Message{{Role: provider.RoleUser, Content: "reply with exactly: PONG"}},
			MaxTokens: 128,
			SessionID: "keel-smoke",
		})
		if err != nil {
			fmt.Fprintf(out, "%-14s FAIL %v\n", name, err)
			failed++
			continue
		}
		var usage *provider.Usage
		for ev := range stream {
			switch ev.Type {
			case provider.EventContent:
				text.WriteString(ev.Text)
			case provider.EventUsage:
				usage = ev.Usage
			case provider.EventError:
				err = ev.Err
			}
		}
		got := strings.TrimSpace(text.String())
		if err != nil {
			fmt.Fprintf(out, "%-14s FAIL %v\n", name, err)
			failed++
			continue
		}
		if got == "" {
			fmt.Fprintf(out, "%-14s FAIL empty response\n", name)
			failed++
			continue
		}
		tokens := 0
		if usage != nil {
			tokens = usage.TotalTokens
		}
		fmt.Fprintf(out, "%-14s OK   %q  %dms tokens=%d\n", name, truncate(got, 60), time.Since(start).Milliseconds(), tokens)
	}
	if failed > 0 {
		fmt.Fprintf(errOut, "smoke: %d provider(s) failed\n", failed)
		return 1
	}
	return 0
}

func pick(reg *provider.Registry, name string) (provider.Provider, error) {
	if name == "" {
		p := reg.Default()
		if p == nil {
			return nil, fmt.Errorf("no provider available")
		}
		return p, nil
	}
	p, ok := reg.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown provider %q (have: %s)", name, strings.Join(reg.Names(), ", "))
	}
	return p, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func oneLine(s string, n int) string {
	return truncate(strings.ReplaceAll(strings.TrimSpace(s), "\n", " "), n)
}

// recordUsage writes one ledger line. Failures are ignored: accounting must
// never break a conversation.
func recordUsage(providerName, model, sessionID string, u *provider.Usage) {
	l, err := usageledger.Open("")
	if err != nil {
		return
	}
	_ = l.Add(usageledger.Record{
		Provider: providerName, Model: model, SessionID: sessionID,
		Prompt: u.PromptTokens, Completion: u.CompletionTokens, Total: u.TotalTokens,
	})
}

func cmdStats(out, errOut io.Writer) int {
	l, err := usageledger.Open("")
	if err != nil {
		fmt.Fprintf(errOut, "stats: %v\n", err)
		return 1
	}
	byProvider, names, err := l.ByProvider()
	if err != nil {
		fmt.Fprintf(errOut, "stats: %v\n", err)
		return 1
	}
	if len(names) == 0 {
		fmt.Fprintln(out, "no usage recorded yet")
		return 0
	}
	fmt.Fprintf(out, "%-16s %8s %10s %10s\n", "provider", "calls", "input", "output")
	for _, n := range names {
		t := byProvider[n]
		fmt.Fprintf(out, "%-16s %8d %10d %10d\n", n, t.Calls, t.Prompt, t.Completion)
	}
	bySession, sessions, _ := l.BySession()
	fmt.Fprintf(out, "\nsessions: %d\n", len(sessions))
	for _, s := range sessions {
		t := bySession[s]
		fmt.Fprintf(out, "  %-18s calls=%-4d tokens=%-8d\n", s, t.Calls, t.Total)
	}
	return 0
}

func cmdTickets(out, errOut io.Writer) int {
	st, err := ticket.OpenStore("")
	if err != nil {
		fmt.Fprintf(errOut, "tickets: %v\n", err)
		return 1
	}
	list, err := st.List()
	if err != nil {
		fmt.Fprintf(errOut, "tickets: %v\n", err)
		return 1
	}
	if len(list) == 0 {
		fmt.Fprintln(out, "no tickets yet")
		return 0
	}
	for _, t := range list {
		fmt.Fprintf(out, "%-10s %-16s %s\n", t.ID, t.State, t.Title)
	}
	return 0
}

// cmdPanel answers the three supervisor questions.
func cmdPanel(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("panel", flag.ContinueOnError)
	fs.SetOutput(errOut)
	historyFor := fs.String("ticket", "", "show the history of one ticket")
	asJSON := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	st, err := ticket.OpenStore("")
	if err != nil {
		fmt.Fprintf(errOut, "panel: %v\n", err)
		return 1
	}
	list, err := st.List()
	if err != nil {
		fmt.Fprintf(errOut, "panel: %v\n", err)
		return 1
	}
	running, queued := 0, 0
	for _, t := range list {
		switch t.State {
		case ticket.Working:
			running++
		case ticket.Queued:
			queued++
		}
	}
	snap := panel.Build(list, running, queued, *historyFor)

	if *asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		_ = enc.Encode(snap)
		return 0
	}

	fmt.Fprintf(out, "1) 需要我处理的工单：%d\n", len(snap.NeedsMe))
	for _, t := range snap.NeedsMe {
		fmt.Fprintf(out, "   %s  %-16s %s\n", t.ID, t.State, t.Title)
	}
	fmt.Fprintf(out, "\n2) 系统健康\n")
	fmt.Fprintf(out, "   运行中 %d  排队 %d\n", snap.Health.Running, snap.Health.Queued)
	for state, n := range snap.Health.ByState {
		fmt.Fprintf(out, "   %-16s %d\n", state, n)
	}
	if *historyFor != "" {
		fmt.Fprintf(out, "\n3) 工单 %s 的历史：%d 条\n", *historyFor, len(snap.History))
		for _, r := range snap.History {
			fmt.Fprintf(out, "   %s  %-8s %s -> %s  %s\n",
				time.Unix(r.At, 0).Format("01-02 15:04"), r.Event, r.From, r.To, r.Note)
		}
	} else {
		fmt.Fprintf(out, "\n3) 用 -ticket <id> 查看单条工单历史\n")
	}
	return 0
}

// cmdServe runs the browser shell over the same core service.
func cmdServe(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(errOut)
	addr := fs.String("addr", "127.0.0.1:8787", "listen address")
	assetsDir := fs.String("assets", "frontend/dist", "built frontend directory")
	logJSON := fs.Bool("log-json", false, "emit JSON lines instead of text logs")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var assets fsimpl.FS
	if st, err := os.Stat(*assetsDir); err == nil && st.IsDir() {
		assets = os.DirFS(*assetsDir)
	} else {
		fmt.Fprintf(errOut, "serve: %s not found; run `cd frontend && npm run build`\n", *assetsDir)
	}

	obs.Init(errOut, *logJSON)
	log := obs.With("serve")

	svc := core.New()
	svc.Bootstrap()
	defer svc.Close()

	srv := httpserver.New(svc, assets)
	log.Info("listening", "addr", *addr, "assets", *assetsDir)
	fmt.Fprintf(out, "keel listening on http://%s\n", *addr)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := appserver.Run(ctx, *addr, srv.Handler(), appserver.DefaultShutdownGrace); err != nil {
		log.Error("serve failed", "err", err.Error())
		fmt.Fprintf(errOut, "serve: %v\n", err)
		return 1
	}
	log.Info("stopped")
	return 0
}

// cmdBackup copies the state files into a timestamped directory.
func cmdBackup(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: keel backup <dir>")
		return 2
	}
	dir, err := backup.Create(args[0], "")
	if err != nil {
		fmt.Fprintf(errOut, "backup: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "backup written to %s\n", dir)
	return 0
}

// cmdRestore copies a backup back over the state directory.
func cmdRestore(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: keel restore <backup-dir>")
		return 2
	}
	n, err := backup.Restore(args[0], "")
	if err != nil {
		fmt.Fprintf(errOut, "restore: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "restored %d files\n", n)
	return 0
}

// cmdWorkflow runs a configuration-defined DAG.
func cmdWorkflow(args []string, out, errOut io.Writer) int {
	if len(args) == 0 || args[0] != "run" {
		fmt.Fprintln(errOut, "usage: keel workflow run <file.json>")
		return 2
	}
	fs := flag.NewFlagSet("workflow run", flag.ContinueOnError)
	fs.SetOutput(errOut)
	providerName := fs.String("provider", "", "provider name")
	model := fs.String("model", "", "model id")
	rest := args[1:]
	if err := fs.Parse(rest); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(errOut, "workflow run: file required")
		return 2
	}
	file := fs.Arg(0)

	def, err := workflow.Load(file)
	if err != nil {
		fmt.Fprintf(errOut, "workflow: %v\n", err)
		return 1
	}

	reg, err := config.Load()
	if err != nil {
		fmt.Fprintf(errOut, "workflow: %v\n", err)
		return 1
	}
	p, err := pick(reg, *providerName)
	if err != nil {
		fmt.Fprintf(errOut, "workflow: %v\n", err)
		return 1
	}

	agentFn := func(ctx context.Context, prompt string) (string, error) {
		stream, err := p.Stream(ctx, provider.Request{
			Model:     *model,
			SessionID: "workflow-" + def.ID,
			Messages:  []provider.Message{{Role: provider.RoleSystem, Content: "Return only the requested result."}, {Role: provider.RoleUser, Content: prompt}},
			MaxTokens: 1024,
		})
		if err != nil {
			return "", err
		}
		var b strings.Builder
		for ev := range stream {
			switch ev.Type {
			case provider.EventContent:
				b.WriteString(ev.Text)
			case provider.EventError:
				if ev.Err != nil {
					return "", ev.Err
				}
			}
		}
		return strings.TrimSpace(b.String()), nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	run := workflow.Execute(ctx, def, workflow.Deps{Agent: agentFn})
	for _, n := range run.Nodes {
		status := n.Status
		fmt.Fprintf(out, "%-5s %-12s %-8s attempts=%d\n", status, n.ID, n.Type, n.Attempts)
		if n.Error != "" {
			fmt.Fprintf(out, "      error: %s\n", oneLine(n.Error, 160))
		}
	}
	fmt.Fprintf(out, "\nworkflow %s: %s (%d nodes)\n", run.Workflow, run.Status, len(run.Nodes))

	if st, err := workflow.OpenStore(""); err == nil {
		if err := st.Save(*run); err != nil {
			fmt.Fprintf(errOut, "workflow: save run: %v\n", err)
		}
	}
	if run.Status != "passed" {
		return 1
	}
	return 0
}

func cmdRuns(out, errOut io.Writer) int {
	st, err := workflow.OpenStore("")
	if err != nil {
		fmt.Fprintf(errOut, "runs: %v\n", err)
		return 1
	}
	list, err := st.List()
	if err != nil {
		fmt.Fprintf(errOut, "runs: %v\n", err)
		return 1
	}
	if len(list) == 0 {
		fmt.Fprintln(out, "no runs yet")
		return 0
	}
	for _, r := range list {
		fmt.Fprintf(out, "%-9s %-16s %d nodes  %s\n", r.Status, r.Workflow, len(r.Nodes),
			time.Unix(r.Ended, 0).Format("01-02 15:04"))
	}
	return 0
}

// cmdHealthcheck is the container liveness probe: it asks the local server.
func cmdHealthcheck(out, errOut io.Writer) int {
	addr := os.Getenv("KEEL_HEALTH_URL")
	if addr == "" {
		addr = "http://127.0.0.1:8787/healthz"
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(addr)
	if err != nil {
		fmt.Fprintf(errOut, "healthcheck: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Fprintf(errOut, "healthcheck: status %d\n", resp.StatusCode)
		return 1
	}
	return 0
}
