package ticket

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Store keeps tickets in one JSON file. Rewriting the whole file is fine at
// this scale and keeps the format readable.
type Store struct {
	mu   sync.Mutex
	path string
}

// DefaultPath returns the ticket file location, honouring AGENTDESK_TICKETS.
func DefaultPath() string {
	if v := os.Getenv("AGENTDESK_TICKETS"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "tickets.json"
	}
	return filepath.Join(home, ".agentdesk", "tickets.json")
}

// OpenStore prepares the ticket file.
func OpenStore(path string) (*Store, error) {
	if path == "" {
		path = DefaultPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return &Store{path: path}, nil
}

// List returns every ticket.
func (s *Store) List() ([]Ticket, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

func (s *Store) read() ([]Ticket, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Ticket
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Save upserts one ticket by id.
func (s *Store) Save(t Ticket) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.read()
	if err != nil {
		return err
	}
	replaced := false
	for i := range list {
		if list[i].ID == t.ID {
			list[i] = t
			replaced = true
			break
		}
	}
	if !replaced {
		list = append(list, t)
	}
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(raw, '\n'), 0o600)
}

// ByDedupKey finds an existing ticket for a trigger key.
func (s *Store) ByDedupKey(key string) (*Ticket, error) {
	list, err := s.List()
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].DedupKey == key {
			return &list[i], nil
		}
	}
	return nil, nil
}
