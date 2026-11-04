// Package ticket is the unit of authorized work: one state machine, one
// history, and a small slot pool that decides what actually runs.
package ticket

import (
	"errors"
	"fmt"
	"time"
)

// State is a ticket stage.
type State string

const (
	Open           State = "open"
	Queued         State = "queued"
	Working        State = "working"
	AwaitingPerson State = "awaiting_person"
	AwaitingReview State = "awaiting_review"
	Ended          State = "ended"
	Failed         State = "failed"
)

// Event names a transition.
type Event string

const (
	Start  Event = "start"
	Queue  Event = "queue"
	Ask    Event = "ask"
	Answer Event = "answer"
	Submit Event = "submit"
	Accept Event = "accept"
	Reject Event = "reject"
	Fail   Event = "fail"
	Retry  Event = "retry"
	Stop   Event = "stop"
)

// ErrIllegal is returned for a transition the machine does not allow.
var ErrIllegal = errors.New("ticket: illegal transition")

type rule struct {
	from State
	ev   Event
	to   State
}

var rules = []rule{
	{Open, Start, Working},
	{Open, Queue, Queued},
	{Queued, Start, Working},
	{Working, Ask, AwaitingPerson},
	{AwaitingPerson, Answer, Working},
	{Working, Submit, AwaitingReview},
	{AwaitingReview, Accept, Ended},
	{AwaitingReview, Reject, Working},
	{Working, Fail, Failed},
	{Failed, Retry, Queued},
	{AwaitingPerson, Stop, Ended},
}

// Record is one history entry.
type Record struct {
	At    int64  `json:"at"`
	Event Event  `json:"event"`
	From  State  `json:"from"`
	To    State  `json:"to"`
	Note  string `json:"note,omitempty"`
}

// Ticket owns its state; it never follows anything else's.
type Ticket struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Source   string   `json:"source"`
	DedupKey string   `json:"dedup_key"`
	State    State    `json:"state"`
	Result   string   `json:"result,omitempty"`
	History  []Record `json:"history"`
}

// New creates an open ticket.
func New(id, title, source, dedupKey string) *Ticket {
	t := &Ticket{ID: id, Title: title, Source: source, DedupKey: dedupKey, State: Open}
	t.History = append(t.History, Record{At: time.Now().Unix(), Event: "created", To: Open})
	return t
}

// Apply moves the ticket along the machine. An illegal move leaves it alone.
func (t *Ticket) Apply(ev Event, note string) error {
	for _, r := range rules {
		if r.from == t.State && r.ev == ev {
			t.History = append(t.History, Record{
				At: time.Now().Unix(), Event: ev, From: t.State, To: r.to, Note: note,
			})
			t.State = r.to
			return nil
		}
	}
	return fmt.Errorf("%w: %s --%s-->", ErrIllegal, t.State, ev)
}

// Can reports whether a transition is allowed right now.
func (t *Ticket) Can(ev Event) bool {
	for _, r := range rules {
		if r.from == t.State && r.ev == ev {
			return true
		}
	}
	return false
}

// Terminal reports whether the ticket is finished.
func (t *Ticket) Terminal() bool { return t.State == Ended }
