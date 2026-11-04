package panel

import (
	"testing"

	"github.com/justin-mc-lai/agentdesk/internal/ticket"
)

func TestPanelAnswersThreeQuestions(t *testing.T) {
	needs := ticket.New("t1", "needs review", "manual", "k1")
	_ = needs.Apply(ticket.Start, "")
	_ = needs.Apply(ticket.Submit, "")

	running := ticket.New("t2", "busy", "manual", "k2")
	_ = running.Apply(ticket.Start, "")

	done := ticket.New("t3", "done", "manual", "k3")
	_ = done.Apply(ticket.Start, "")
	_ = done.Apply(ticket.Submit, "")
	_ = done.Apply(ticket.Accept, "")

	snap := Build([]ticket.Ticket{*needs, *running, *done}, 1, 0, "t1")

	if len(snap.NeedsMe) != 1 || snap.NeedsMe[0].ID != "t1" {
		t.Fatalf("needs me = %+v", snap.NeedsMe)
	}
	if snap.Health.ByState[ticket.Working] != 1 || snap.Health.ByState[ticket.Ended] != 1 {
		t.Fatalf("health = %+v", snap.Health)
	}
	if snap.Health.Running != 1 {
		t.Fatalf("running = %d", snap.Health.Running)
	}
	if len(snap.History) != 3 {
		t.Fatalf("history rows = %d", len(snap.History))
	}
}
