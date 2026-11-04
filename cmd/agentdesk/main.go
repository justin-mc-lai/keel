// Command agentdesk is the headless entry point. The desktop app and this CLI
// share the same provider, tool, store and agent packages; only the transport
// differs (stdout here, Wails events in the app).
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/justin-mc-lai/agentdesk/internal/agent"
	"github.com/justin-mc-lai/agentdesk/internal/config"
	"github.com/justin-mc-lai/agentdesk/internal/provider"
	"github.com/justin-mc-lai/agentdesk/internal/store"
	"github.com/justin-mc-lai/agentdesk/internal/tool"
	usageledger "github.com/justin-mc-lai/agentdesk/internal/usage"
	"github.com/justin-mc-lai/agentdesk/internal/workspace"
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
	case "version", "-v", "--version":
		fmt.Fprintln(out, "agentdesk "+Version)
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
	fmt.Fprint(w, `agentdesk — a provider-agnostic desktop LLM agent base

usage:
  agentdesk run -p "prompt" [-provider name] [-model id] [-effort level] [-session id] [-yes]
  agentdesk providers
  agentdesk sessions
  agentdesk history <session-id>
  agentdesk smoke
  agentdesk stats
  agentdesk version
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

// runPrompt is the testable core of `agentdesk run`.
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
			SessionID: "agentdesk-smoke",
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
