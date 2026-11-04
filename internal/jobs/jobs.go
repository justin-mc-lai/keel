// Package jobs runs shell commands that outlive one tool call.
//
// A foreground command blocks the agent loop: the model cannot be told
// anything until it returns. Anything slower than a few seconds is detached
// and polled instead, with a hard lifetime so a runaway process cannot sit
// there forever.
package jobs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// DefaultLimit is how long one job may live before it is killed.
const DefaultLimit = 30 * time.Minute

const maxOutput = 8192

type job struct {
	id      string
	command string
	buf     *cappedBuffer
	done    bool
	err     error
	cancel  context.CancelFunc
}

// Manager owns every detached job.
type Manager struct {
	mu    sync.Mutex
	jobs  map[string]*job
	limit time.Duration
}

// New builds a manager with a lifetime bound.
func New(limit time.Duration) *Manager {
	if limit <= 0 {
		limit = DefaultLimit
	}
	return &Manager{jobs: map[string]*job{}, limit: limit}
}

// Start launches a command detached from the caller.
func (m *Manager) Start(command, dir string) (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), m.limit)
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.WaitDelay = 2 * time.Second
	setupProc(cmd)

	rec := &job{id: id, command: command, buf: &cappedBuffer{max: maxOutput}, cancel: cancel}
	cmd.Stdout = rec.buf
	cmd.Stderr = rec.buf
	if err := cmd.Start(); err != nil {
		cancel()
		return "", err
	}

	m.mu.Lock()
	m.jobs[id] = rec
	m.mu.Unlock()

	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		rec.done = true
		rec.err = err
		m.mu.Unlock()
		cancel()
	}()
	return id, nil
}

// Poll returns the current output and whether the job has finished.
func (m *Manager) Poll(id string) (string, bool, error) {
	m.mu.Lock()
	rec, ok := m.jobs[id]
	if !ok {
		m.mu.Unlock()
		return "", false, fmt.Errorf("jobs: unknown job %q", id)
	}
	done, err := rec.done, rec.err
	out := rec.buf.String()
	m.mu.Unlock()

	if done && err != nil {
		return out, true, fmt.Errorf("%v: %s", err, out)
	}
	return out, done, nil
}

// Wait polls until the job finishes or the wait window expires.
func (m *Manager) Wait(ctx context.Context, id string, window time.Duration) (string, bool, error) {
	deadline := time.Now().Add(window)
	for {
		out, done, err := m.Poll(id)
		if done || err != nil {
			return out, done, err
		}
		if time.Now().After(deadline) {
			return out, false, nil
		}
		select {
		case <-ctx.Done():
			return out, false, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

// Kill terminates a job and its process group.
func (m *Manager) Kill(id string) error {
	m.mu.Lock()
	rec, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("jobs: unknown job %q", id)
	}
	rec.cancel()
	return nil
}

// List returns live job ids.
func (m *Manager) List() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.jobs))
	for id := range m.jobs {
		out = append(out, id)
	}
	return out
}

func newID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "job_" + hex.EncodeToString(b), nil
}

// cappedBuffer keeps a runaway command from flooding memory.
type cappedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.buf.Len() < c.max {
		room := c.max - c.buf.Len()
		if len(p) > room {
			c.buf.Write(p[:room])
			c.truncated = true
		} else {
			c.buf.Write(p)
		}
	} else if len(p) > 0 {
		c.truncated = true
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string { return c.buf.String() }
