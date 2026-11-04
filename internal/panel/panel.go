// Package panel answers the three questions a supervisor actually asks.
package panel

import (
	"sort"
	"time"

	"github.com/justin-mc-lai/agentdesk/internal/ticket"
)

// Health is the second question: is the system doing the right thing.
type Health struct {
	ByState map[ticket.State]int `json:"by_state"`
	Running int                  `json:"running"`
	Queued  int                  `json:"queued"`
}

// Snapshot is one read of the panel.
type Snapshot struct {
	NeedsMe []ticket.Ticket `json:"needs_me"`
	Health  Health          `json:"health"`
	History []ticket.Record `json:"history"`
	At      int64           `json:"at"`
}

// Build answers the three questions over a set of tickets.
//
// "Needs me" is deliberately narrow: only tickets a person can unblock. A
// ticket that is merely queued is not the human's problem.
func Build(tickets []ticket.Ticket, running, queued int, historyFor string) Snapshot {
	out := Snapshot{
		Health: Health{ByState: map[ticket.State]int{}, Running: running, Queued: queued},
		At:     time.Now().Unix(),
	}
	for _, t := range tickets {
		out.Health.ByState[t.State]++
		switch t.State {
		case ticket.AwaitingPerson, ticket.AwaitingReview, ticket.Failed:
			out.NeedsMe = append(out.NeedsMe, t)
		}
		if historyFor != "" && t.ID == historyFor {
			out.History = append(out.History, t.History...)
		}
	}
	sort.Slice(out.NeedsMe, func(i, j int) bool { return out.NeedsMe[i].ID < out.NeedsMe[j].ID })
	return out
}
