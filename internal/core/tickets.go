package core

import (
	"fmt"

	"github.com/justin-mc-lai/agentdesk/internal/panel"
	"github.com/justin-mc-lai/agentdesk/internal/ticket"
)

// ticketStore opens the ticket file. It is cheap: the file is small.
func (s *Service) ticketStore() (*ticket.Store, error) {
	return ticket.OpenStore("")
}

// Tickets returns every ticket for the board.
func (s *Service) Tickets() ([]ticket.Ticket, error) {
	st, err := s.ticketStore()
	if err != nil {
		return nil, err
	}
	list, err := st.List()
	if err != nil {
		return nil, err
	}
	if list == nil {
		return []ticket.Ticket{}, nil
	}
	return list, nil
}

// PanelSnapshot answers the three supervisor questions from real tickets.
func (s *Service) PanelSnapshot(historyFor string) (panel.Snapshot, error) {
	list, err := s.Tickets()
	if err != nil {
		return panel.Snapshot{}, err
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
	return panel.Build(list, running, queued, historyFor), nil
}

// actionEvents maps a UI action onto the ticket machine's event.
var actionEvents = map[string]ticket.Event{
	"accept": ticket.Accept,
	"reject": ticket.Reject,
	"retry":  ticket.Retry,
	"start":  ticket.Start,
	"stop":   ticket.Stop,
}

// ApplyTicketAction advances one ticket. Illegal actions return an error and
// leave the stored ticket untouched.
func (s *Service) ApplyTicketAction(id, action string) (ticket.Ticket, error) {
	ev, ok := actionEvents[action]
	if !ok {
		return ticket.Ticket{}, fmt.Errorf("unknown action %q", action)
	}
	st, err := s.ticketStore()
	if err != nil {
		return ticket.Ticket{}, err
	}
	list, err := st.List()
	if err != nil {
		return ticket.Ticket{}, err
	}
	for i := range list {
		if list[i].ID != id {
			continue
		}
		if err := list[i].Apply(ev, "ui"); err != nil {
			return list[i], err
		}
		if err := st.Save(list[i]); err != nil {
			return list[i], err
		}
		return list[i], nil
	}
	return ticket.Ticket{}, fmt.Errorf("ticket %q not found", id)
}
