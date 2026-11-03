package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/justin-mc-lai/agentdesk/internal/provider"
	"github.com/justin-mc-lai/agentdesk/internal/store"
	"github.com/justin-mc-lai/agentdesk/internal/tool"
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

	tools, err := tool.NewDefaultRegistry(nil)
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
	tools, _ := tool.NewDefaultRegistry(nil)
	sid, _ := st.CreateSession("t")

	loop := New(Options{Provider: p, Tools: tools, Store: st, MaxIterations: 3})
	_, err = loop.Run(context.Background(), sid, "loop")
	if err == nil || !strings.Contains(err.Error(), "exceeded") {
		t.Fatalf("want iteration error, got %v", err)
	}
}
