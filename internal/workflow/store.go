package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Store keeps run records in one JSON file.
type Store struct {
	mu   sync.Mutex
	path string
}

// DefaultPath returns the run file, honouring AGENTDESK_RUNS.
func DefaultPath() string {
	if v := os.Getenv("AGENTDESK_RUNS"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "runs.json"
	}
	return filepath.Join(home, ".agentdesk", "runs.json")
}

// OpenStore prepares the run file.
func OpenStore(path string) (*Store, error) {
	if path == "" {
		path = DefaultPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return &Store{path: path}, nil
}

// List returns every run, oldest first.
func (s *Store) List() ([]Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

func (s *Store) read() ([]Run, error) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Run
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Save appends one run.
func (s *Store) Save(r Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.read()
	if err != nil {
		return err
	}
	list = append(list, r)
	raw, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, append(raw, '\n'), 0o600)
}
