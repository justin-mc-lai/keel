package core

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/justin-mc-lai/agentdesk/internal/provider"
	"github.com/justin-mc-lai/agentdesk/internal/store"
)

type fakeProvider struct{}

func (fakeProvider) Name() string         { return "fake" }
func (fakeProvider) DefaultModel() string { return "fake-1" }
func (fakeProvider) Models() []string     { return []string{"fake-1"} }
func (fakeProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	ch := make(chan provider.Event, 4)
	go func() {
		defer close(ch)
		ch <- provider.Event{Type: provider.EventContent, Text: "hello "}
		ch <- provider.Event{Type: provider.EventContent, Text: "world"}
		ch <- provider.Event{Type: provider.EventUsage, Usage: &provider.Usage{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5}}
		ch <- provider.Event{Type: provider.EventDone}
	}()
	return ch, nil
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	reg := provider.NewRegistry()
	reg.Add(fakeProvider{})
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return NewWith(reg, st)
}

func TestServiceIsTransportNeutral(t *testing.T) {
	svc := newTestService(t)

	var mu sync.Mutex
	run := func() []string {
		var seen []string
		sink := func(name string, data any) {
			switch v := data.(type) {
			case provider.Event:
				seen = append(seen, string(v.Type)+":"+v.Text)
			default:
				seen = append(seen, name)
			}
		}
		id, err := svc.NewSession()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Send(context.Background(), SendRequest{
			SessionID: id, Provider: "fake", Text: "hi",
		}, sink); err != nil {
			t.Fatalf("send: %v", err)
		}
		mu.Lock()
		defer mu.Unlock()
		return seen
	}

	a := run()
	b := run()
	if len(a) != len(b) {
		t.Fatalf("event counts differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("event %d differs: %q vs %q", i, a[i], b[i])
		}
	}
	if len(a) < 3 {
		t.Fatalf("too few events: %v", a)
	}

	// The service is usable without any transport attached.
	if state := svc.PermissionState(); state.Mode != "ask" {
		t.Fatalf("default mode = %q", state.Mode)
	}
}
