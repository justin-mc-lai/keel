// Package trigger turns plans and events into idempotent triggers.
//
// The hard requirement is that a restart must not replay a scheduled instant.
// Every trigger carries a dedup key; the ledger refuses a key it has seen.
package trigger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Trigger is one immutable event.
type Trigger struct {
	ID       string `json:"id"`
	Source   string `json:"source"`
	Kind     string `json:"kind"`
	DedupKey string `json:"dedup_key"`
	Prompt   string `json:"prompt"`
	At       int64  `json:"at"`
}

// Schedule is one plan.
type Schedule struct {
	ID      string `json:"id"`
	Plan    string `json:"plan"` // "@every 30s" or "daily 09:00"
	Prompt  string `json:"prompt"`
	Enabled bool   `json:"enabled"`
}

// Ledger persists triggers and the dedup keys already used.
type Ledger struct {
	mu   sync.Mutex
	path string
	seen map[string]bool
	all  []Trigger
}

// OpenLedger loads the ledger file, rebuilding the dedup set.
func OpenLedger(path string) (*Ledger, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, ".keel", "triggers.jsonl")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	l := &Ledger{path: path, seen: map[string]bool{}}
	f, err := os.Open(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		return l, nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var t Trigger
		if json.Unmarshal(sc.Bytes(), &t) != nil {
			continue
		}
		l.all = append(l.all, t)
		l.seen[t.DedupKey] = true
	}
	return l, sc.Err()
}

// Emit records a trigger. It returns false when the dedup key was already used.
func (l *Ledger) Emit(t Trigger) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if t.DedupKey != "" && l.seen[t.DedupKey] {
		return false, nil
	}
	if t.At == 0 {
		t.At = time.Now().Unix()
	}
	line, err := json.Marshal(t)
	if err != nil {
		return false, err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return false, err
	}
	if t.DedupKey != "" {
		l.seen[t.DedupKey] = true
	}
	l.all = append(l.all, t)
	return true, nil
}

// List returns every trigger in arrival order.
func (l *Ledger) List() []Trigger {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Trigger(nil), l.all...)
}

// Due computes the triggers a schedule owes at now. Already-emitted instants
// are filtered by the ledger, so calling this twice is safe.
func (l *Ledger) Due(s Schedule, now time.Time) ([]Trigger, error) {
	if !s.Enabled {
		return nil, nil
	}
	instant, err := fireTime(s.Plan, now)
	if err != nil {
		return nil, err
	}
	if instant.IsZero() || instant.After(now) {
		return nil, nil
	}
	key := s.ID + "@" + strconv.FormatInt(instant.Unix(), 10)
	l.mu.Lock()
	seen := l.seen[key]
	l.mu.Unlock()
	if seen {
		return nil, nil
	}
	return []Trigger{{
		ID:       key,
		Source:   s.ID,
		Kind:     "schedule",
		DedupKey: key,
		Prompt:   s.Prompt,
		At:       instant.Unix(),
	}}, nil
}

// fireTime is the most recent instant the plan should have fired at.
func fireTime(plan string, now time.Time) (time.Time, error) {
	plan = strings.TrimSpace(plan)
	switch {
	case strings.HasPrefix(plan, "@every "):
		d, err := time.ParseDuration(strings.TrimSpace(strings.TrimPrefix(plan, "@every ")))
		if err != nil || d <= 0 {
			return time.Time{}, fmt.Errorf("trigger: bad @every duration %q", plan)
		}
		return time.Unix(now.Unix()/int64(d.Seconds())*int64(d.Seconds()), 0), nil
	case strings.HasPrefix(plan, "daily "):
		hhmm := strings.TrimSpace(strings.TrimPrefix(plan, "daily "))
		parts := strings.Split(hhmm, ":")
		if len(parts) != 2 {
			return time.Time{}, fmt.Errorf("trigger: bad daily time %q", plan)
		}
		h, err1 := strconv.Atoi(parts[0])
		m, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
			return time.Time{}, fmt.Errorf("trigger: bad daily time %q", plan)
		}
		at := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
		if at.After(now) {
			at = at.AddDate(0, 0, -1)
		}
		return at, nil
	default:
		return time.Time{}, fmt.Errorf("trigger: unknown plan %q", plan)
	}
}
