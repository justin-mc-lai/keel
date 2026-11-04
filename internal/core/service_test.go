package core

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/justin-mc-lai/keel/internal/provider"
	"github.com/justin-mc-lai/keel/internal/store"
	"github.com/justin-mc-lai/keel/internal/ticket"
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

func seedTicket(t *testing.T, id string, apply ...ticket.Event) {
	t.Helper()
	st, err := ticket.OpenStore("")
	if err != nil {
		t.Fatal(err)
	}
	tk := ticket.New(id, "ticket "+id, "manual", "k-"+id)
	for _, ev := range apply {
		if err := tk.Apply(ev, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Save(*tk); err != nil {
		t.Fatal(err)
	}
}

func isolatedTickets(t *testing.T) {
	t.Helper()
	t.Setenv("KEEL_TICKETS", filepath.Join(t.TempDir(), "tickets.json"))
}

func TestTicketsListAndHistory(t *testing.T) {
	isolatedTickets(t)
	svc := newTestService(t)
	seedTicket(t, "t-501a", ticket.Start, ticket.Submit)
	seedTicket(t, "t-501b", ticket.Start)

	list, err := svc.Tickets()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("tickets = %d", len(list))
	}
	var review *ticket.Ticket
	for i := range list {
		if list[i].ID == "t-501a" {
			review = &list[i]
		}
	}
	if review == nil || review.State != ticket.AwaitingReview {
		t.Fatalf("expected awaiting_review, got %+v", review)
	}
	if len(review.History) != 3 {
		t.Fatalf("history = %d", len(review.History))
	}
}

func TestPanelSnapshotFromTickets(t *testing.T) {
	isolatedTickets(t)
	svc := newTestService(t)
	seedTicket(t, "t-503a", ticket.Start, ticket.Submit)
	seedTicket(t, "t-503b", ticket.Start)

	snap, err := svc.PanelSnapshot("t-503a")
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.NeedsMe) != 1 || snap.NeedsMe[0].ID != "t-503a" {
		t.Fatalf("needs me = %+v", snap.NeedsMe)
	}
	if snap.Health.Running != 1 {
		t.Fatalf("running = %d", snap.Health.Running)
	}
	if len(snap.History) == 0 {
		t.Fatal("history missing")
	}
}

func TestTicketActionTransitions(t *testing.T) {
	isolatedTickets(t)
	svc := newTestService(t)
	seedTicket(t, "t-504a", ticket.Start, ticket.Submit)

	got, err := svc.ApplyTicketAction("t-504a", "accept")
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if got.State != ticket.Ended {
		t.Fatalf("state = %s", got.State)
	}
	if _, err := svc.ApplyTicketAction("t-504a", "accept"); err == nil {
		t.Fatal("accept on an ended ticket must fail")
	}
	if _, err := svc.ApplyTicketAction("nope", "accept"); err == nil {
		t.Fatal("unknown ticket must fail")
	}
	if _, err := svc.ApplyTicketAction("t-504a", "explode"); err == nil {
		t.Fatal("unknown action must fail")
	}

	seedTicket(t, "t-504b")
	if _, err := svc.ApplyTicketAction("t-504b", "reject"); err == nil {
		t.Fatal("reject from open must fail")
	}
}
