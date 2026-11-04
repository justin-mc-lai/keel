// Package usage keeps a local ledger of token counts and cost per call.
//
// Append-only JSONL: no schema migration, no dependency, and a corrupt line
// can be skipped without losing the rest of the file.
package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Record is one model call.
type Record struct {
	At         int64   `json:"at"`
	Provider   string  `json:"provider"`
	Model      string  `json:"model"`
	SessionID  string  `json:"session_id"`
	Prompt     int     `json:"prompt"`
	Completion int     `json:"completion"`
	Total      int     `json:"total"`
	Cost       float64 `json:"cost"`
}

// Totals is an aggregate over a group.
type Totals struct {
	Calls      int     `json:"calls"`
	Prompt     int     `json:"prompt"`
	Completion int     `json:"completion"`
	Total      int     `json:"total"`
	Cost       float64 `json:"cost"`
}

func (t *Totals) add(r Record) {
	t.Calls++
	t.Prompt += r.Prompt
	t.Completion += r.Completion
	t.Total += r.Total
	t.Cost += r.Cost
}

// Ledger appends records to one file.
type Ledger struct {
	mu   sync.Mutex
	path string
}

// DefaultPath returns ~/.keel/usage.jsonl.
func DefaultPath() string {
	if v := os.Getenv("KEEL_STATE_DIR"); v != "" {
		return filepath.Join(v, "usage.jsonl")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "usage.jsonl"
	}
	return filepath.Join(home, ".keel", "usage.jsonl")
}

// Open prepares the ledger file.
func Open(path string) (*Ledger, error) {
	if path == "" {
		path = DefaultPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	f.Close()
	return &Ledger{path: path}, nil
}

// Add appends one record.
func (l *Ledger) Add(r Record) error {
	if r.At == 0 {
		r.At = time.Now().Unix()
	}
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

// Records reads the ledger, skipping unparseable lines.
func (l *Ledger) Records() ([]Record, error) {
	f, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var r Record
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

// ByProvider aggregates every record per provider, sorted by name.
func (l *Ledger) ByProvider() (map[string]Totals, []string, error) {
	return l.rollup(func(r Record) string { return r.Provider })
}

// BySession aggregates every record per session, sorted by id.
func (l *Ledger) BySession() (map[string]Totals, []string, error) {
	return l.rollup(func(r Record) string { return r.SessionID })
}

func (l *Ledger) rollup(key func(Record) string) (map[string]Totals, []string, error) {
	recs, err := l.Records()
	if err != nil {
		return nil, nil, err
	}
	out := map[string]Totals{}
	for _, r := range recs {
		k := key(r)
		t := out[k]
		t.add(r)
		out[k] = t
	}
	keys := make([]string, 0, len(out))
	for k := range out {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return out, keys, nil
}
