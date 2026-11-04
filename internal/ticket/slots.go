package ticket

import "sync"

// Slots bounds how many tickets run at once and queues the rest.
type Slots struct {
	mu      sync.Mutex
	limit   int
	running map[string]bool
	queue   []string
}

// NewSlots builds a pool. A non-positive limit means one.
func NewSlots(limit int) *Slots {
	if limit <= 0 {
		limit = 1
	}
	return &Slots{limit: limit, running: map[string]bool{}}
}

// Acquire takes a slot. It returns true when the ticket may start now, and
// false when it was queued.
func (s *Slots) Acquire(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, already := s.running[id]; already {
		return true
	}
	if len(s.running) < s.limit {
		s.running[id] = true
		return true
	}
	for _, q := range s.queue {
		if q == id {
			return false
		}
	}
	s.queue = append(s.queue, id)
	return false
}

// Release frees a slot and returns the next queued ticket, if any.
func (s *Slots) Release(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, id)
	if len(s.running) >= s.limit || len(s.queue) == 0 {
		return "", false
	}
	next := s.queue[0]
	s.queue = s.queue[1:]
	s.running[next] = true
	return next, true
}

// Stats reports current occupancy.
func (s *Slots) Stats() (running, queued int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.running), len(s.queue)
}
