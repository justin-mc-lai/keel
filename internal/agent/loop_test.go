package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/justin-mc-lai/keel/internal/perm"
	"github.com/justin-mc-lai/keel/internal/provider"
	"github.com/justin-mc-lai/keel/internal/store"
	"github.com/justin-mc-lai/keel/internal/tool"
	"github.com/justin-mc-lai/keel/internal/workspace"
)

// scriptedProvider replays a fixed list of streams in order.
type scriptedProvider struct {
	mu    sync.Mutex
	turns [][]provider.Event
	seen  int
}

func (p *scriptedProvider) Name() string         { return "scripted" }
func (p *scriptedProvider) DefaultModel() string { return "fake-1" }
func (p *scriptedProvider) Models() []string     { return []string{"fake-1"} }

func (p *scriptedProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	p.mu.Lock()
	if p.seen >= len(p.turns) {
		p.mu.Unlock()
		return nil, context.DeadlineExceeded
	}
	turn := p.turns[p.seen]
	p.seen++
	p.mu.Unlock()

	ch := make(chan provider.Event, len(turn)+1)
	go func() {
		defer close(ch)
		for _, ev := range turn {
			ch <- ev
		}
		ch <- provider.Event{Type: provider.EventDone}
	}()
	return ch, nil
}

func TestLoopExecutesToolAndConverges(t *testing.T) {
	call := provider.ToolCall{ID: "c1", Name: "echo", Arguments: `{"text":"hi"}`}
	p := &scriptedProvider{turns: [][]provider.Event{
		{
			{Type: provider.EventContent, Text: "let me check"},
			{Type: provider.EventToolCall, ToolCall: &call},
		},
		{
			{Type: provider.EventContent, Text: "final answer"},
		},
	}}

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	tools, err := tool.NewDefaultRegistry(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sid, err := st.CreateSession("t")
	if err != nil {
		t.Fatal(err)
	}

	var sawToolDone bool
	loop := New(Options{
		Provider: p,
		Tools:    tools,
		Store:    st,
		OnEvent: func(ev provider.Event) {
			if ev.Type == provider.EventToolDone {
				sawToolDone = true
			}
		},
	})

	got, err := loop.Run(context.Background(), sid, "say hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "final answer" {
		t.Fatalf("answer = %q", got)
	}
	if p.seen != 2 {
		t.Fatalf("provider calls = %d, want 2", p.seen)
	}
	if !sawToolDone {
		t.Fatal("tool round trip did not complete")
	}

	msgs, err := st.Messages(sid)
	if err != nil {
		t.Fatal(err)
	}
	var toolMsgs int
	for _, m := range msgs {
		if m.Role == provider.RoleTool {
			toolMsgs++
			if m.ToolCallID != "c1" || m.Content != "hi" {
				t.Fatalf("tool message = %+v", m)
			}
		}
	}
	if toolMsgs != 1 {
		t.Fatalf("tool messages = %d", toolMsgs)
	}
}

func TestLoopStopsAtIterationLimit(t *testing.T) {
	call := provider.ToolCall{ID: "c1", Name: "echo", Arguments: `{"text":"hi"}`}
	turns := make([][]provider.Event, 5)
	for i := range turns {
		turns[i] = []provider.Event{{Type: provider.EventToolCall, ToolCall: &call}}
	}
	p := &scriptedProvider{turns: turns}

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tools, _ := tool.NewDefaultRegistry(nil, nil)
	sid, _ := st.CreateSession("t")

	loop := New(Options{Provider: p, Tools: tools, Store: st, MaxIterations: 3})
	_, err = loop.Run(context.Background(), sid, "loop")
	if err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("want iteration error, got %v", err)
	}
}

// gatedTools builds a registry with one tool that requires approval.
func gatedTools(t *testing.T, approve func(ctx context.Context, name string, args []byte) bool) *tool.Registry {
	t.Helper()
	r := tool.NewRegistry(approve)
	if err := r.Register(tool.Tool{
		Name:             "danger",
		RequiresApproval: true,
		Run: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "ran", nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestApprovalBlocksThenResumes(t *testing.T) {
	call := provider.ToolCall{ID: "c1", Name: "danger", Arguments: `{}`}
	p := &scriptedProvider{turns: [][]provider.Event{
		{{Type: provider.EventToolCall, ToolCall: &call}},
		{{Type: provider.EventContent, Text: "done"}},
	}}

	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sid, _ := st.CreateSession("t")

	asked := make(chan string, 1)
	release := make(chan struct{})
	ctrl := perm.New(func(ctx context.Context, sessionID, name string, args []byte) (perm.Decision, error) {
		asked <- name
		<-release
		return perm.AllowOnce, nil
	})

	loop := New(Options{Provider: p, Tools: gatedTools(t, ctrl.Approver(sid)), Store: st})
	done := make(chan struct {
		answer string
		err    error
	}, 1)
	go func() {
		a, e := loop.Run(context.Background(), sid, "do it")
		done <- struct {
			answer string
			err    error
		}{a, e}
	}()

	select {
	case name := <-asked:
		if name != "danger" {
			t.Fatalf("asked for %q", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("approval was never requested")
	}

	// The loop must still be blocked while the human has not answered.
	select {
	case r := <-done:
		t.Fatalf("loop finished before approval: %+v", r)
	case <-time.After(150 * time.Millisecond):
	}

	close(release)

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Run: %v", r.err)
		}
		if r.answer != "done" {
			t.Fatalf("answer = %q", r.answer)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not resume after approval")
	}

	msgs, _ := st.Messages(sid)
	var sawTool bool
	for _, m := range msgs {
		if m.Role == provider.RoleTool {
			sawTool = true
			if m.Content != "ran" {
				t.Fatalf("tool result = %q", m.Content)
			}
		}
	}
	if !sawTool {
		t.Fatal("tool result not persisted")
	}
}

func TestApprovalTimeoutAndCancel(t *testing.T) {
	newLoop := func(t *testing.T, ctrl *perm.Controller) (*Loop, *store.Store, string, *scriptedProvider) {
		call := provider.ToolCall{ID: "c1", Name: "danger", Arguments: `{}`}
		p := &scriptedProvider{turns: [][]provider.Event{
			{{Type: provider.EventToolCall, ToolCall: &call}},
			{{Type: provider.EventContent, Text: "done"}},
		}}
		st, err := store.Open(":memory:")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { st.Close() })
		sid, _ := st.CreateSession("t")
		return New(Options{Provider: p, Tools: gatedTools(t, ctrl.Approver(sid)), Store: st}), st, sid, p
	}

	t.Run("timeout denies and the turn continues", func(t *testing.T) {
		ctrl := perm.New(func(ctx context.Context, sessionID, name string, args []byte) (perm.Decision, error) {
			<-ctx.Done()
			return perm.Deny, ctx.Err()
		})
		ctrl.SetTimeout(50 * time.Millisecond)
		loop, st, sid, _ := newLoop(t, ctrl)

		start := time.Now()
		answer, err := loop.Run(context.Background(), sid, "go")
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("timeout took %v", elapsed)
		}
		if answer != "done" {
			t.Fatalf("answer = %q", answer)
		}
		msgs, _ := st.Messages(sid)
		var denied bool
		for _, m := range msgs {
			if m.Role == provider.RoleTool && m.Content == "error: tool: approval denied" {
				denied = true
			}
		}
		if !denied {
			t.Fatal("tool result should record the denial")
		}
	})

	t.Run("cancel unblocks the pending approval", func(t *testing.T) {
		ctrl := perm.New(func(ctx context.Context, sessionID, name string, args []byte) (perm.Decision, error) {
			<-ctx.Done()
			return perm.Deny, ctx.Err()
		})
		ctrl.SetTimeout(30 * time.Second)
		loop, _, sid, _ := newLoop(t, ctrl)

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- func() error { _, e := loop.Run(ctx, sid, "go"); return e }() }()

		time.Sleep(100 * time.Millisecond)
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("cancel did not unblock the loop")
		}
	})
}

func TestAttachmentExpansion(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("secret sauce"), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}

	got := ExpandAttachments(ws, "please read @note.txt")
	if !strings.Contains(got, "secret sauce") {
		t.Fatalf("attachment not inlined: %q", got)
	}
	if !strings.Contains(got, `path="note.txt"`) {
		t.Fatalf("path missing from block: %q", got)
	}

	// A missing path and an email must pass through untouched.
	plain := "mail me at a@b.com about @nope.txt"
	if out := ExpandAttachments(ws, plain); out != plain {
		t.Fatalf("non-attachment rewritten: %q", out)
	}
}

func TestSubagentIsolatedSession(t *testing.T) {
	parent := &scriptedProvider{turns: [][]provider.Event{
		{{Type: provider.EventContent, Text: "parent answer"}},
	}}
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	pid, _ := st.CreateSession("parent")

	tools, _ := tool.NewDefaultRegistry(nil, nil)
	loop := New(Options{Provider: parent, Tools: tools, Store: st})

	child := loop.Subagent()
	if _, err := child(context.Background(), "do the child thing"); err != nil {
		t.Fatalf("child: %v", err)
	}

	// The parent session must not gain any message from the child run.
	parentMsgs, err := st.Messages(pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(parentMsgs) != 0 {
		t.Fatalf("parent session polluted: %+v", parentMsgs)
	}

	// The child got a session of its own with its own user message.
	list, err := st.ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("sessions = %d, want 2", len(list))
	}
	var childID string
	for _, s := range list {
		if s.ID != pid {
			childID = s.ID
		}
	}
	msgs, _ := st.Messages(childID)
	if len(msgs) != 2 {
		t.Fatalf("child messages = %d, want user+assistant", len(msgs))
	}
	if msgs[0].Content != "do the child thing" || msgs[1].Content != "parent answer" {
		t.Fatalf("child history = %+v", msgs)
	}
	if strings.Contains(msgs[0].Content, "parent") {
		t.Fatal("child saw parent context")
	}
}
