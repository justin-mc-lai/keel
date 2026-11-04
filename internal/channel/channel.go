// Package channel writes a finished ticket's outcome back somewhere the
// originator can see it. The interface is the point; the local file is the
// default implementation until a real GitHub or IM adapter exists.
package channel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Record is one write-back.
type Record struct {
	TicketID string `json:"ticket_id"`
	Source   string `json:"source"`
	Result   string `json:"result"`
	At       int64  `json:"at"`
}

// Channel is the write side of a conversation.
type Channel interface {
	Write(r Record) error
}

// Local appends records to a JSONL file.
type Local struct {
	mu   sync.Mutex
	path string
}

// NewLocal opens the default local channel.
func NewLocal(path string) (*Local, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, ".agentdesk", "channel.jsonl")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return &Local{path: path}, nil
}

// Write appends one record.
func (l *Local) Write(r Record) error {
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
	_, err = f.Write(append(line, '\n'))
	return err
}

// Records reads everything written so far.
func (l *Local) Records() ([]Record, error) {
	raw, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Record
	for _, line := range splitLines(raw) {
		var r Record
		if json.Unmarshal(line, &r) == nil {
			out = append(out, r)
		}
	}
	return out, nil
}

func splitLines(raw []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, b := range raw {
		if b == '\n' {
			if i > start {
				out = append(out, raw[start:i])
			}
			start = i + 1
		}
	}
	if start < len(raw) {
		out = append(out, raw[start:])
	}
	return out
}
