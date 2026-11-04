// Package perm decides whether a tool call may run.
//
// Three modes, matching the habit users already have from Codex and Claude
// Code:
//
//	ask     ask every time (default, safest)
//	allow   ask once per tool per session, then remember
//	bypass  never ask (explicit, and surfaced as a warning)
//
// The window between "ask" and "answer" is the dangerous part: it blocks the
// agent loop, so every ask is bounded by a timeout and by the turn context.
package perm

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/justin-mc-lai/keel/internal/tool"
)

// Mode is the permission tier.
type Mode string

const (
	// Ask prompts for every call that requires approval.
	Ask Mode = "ask"
	// Allow prompts once per tool per session, then remembers the answer.
	Allow Mode = "allow"
	// Bypass never prompts.
	Bypass Mode = "bypass"
)

// Decision is the answer to one approval request.
type Decision string

const (
	AllowOnce    Decision = "allow_once"
	AllowSession Decision = "allow_session"
	Deny         Decision = "deny"
)

// DefaultTimeout bounds how long a single approval request may block.
const DefaultTimeout = 120 * time.Second

// Asker asks the human. It must return as soon as ctx is done.
type Asker func(ctx context.Context, sessionID, tool string, args []byte) (Decision, error)

// Controller holds the mode and the per-session allow memory.
type Controller struct {
	mu      sync.RWMutex
	mode    Mode
	allowed map[string]map[string]bool
	ask     Asker
	timeout time.Duration
}

// New builds a controller. A nil asker means every request in ask/allow mode
// is denied — the safe default.
func New(ask Asker) *Controller {
	return &Controller{
		mode:    Ask,
		allowed: map[string]map[string]bool{},
		ask:     ask,
		timeout: DefaultTimeout,
	}
}

// Mode returns the current tier.
func (c *Controller) Mode() Mode {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.mode
}

// SetMode switches the tier. Unknown values are rejected.
func (c *Controller) SetMode(m Mode) error {
	switch m {
	case Ask, Allow, Bypass:
	default:
		return ErrUnknownMode
	}
	c.mu.Lock()
	c.mode = m
	c.mu.Unlock()
	return nil
}

// SetTimeout overrides the approval window (tests use a short one).
func (c *Controller) SetTimeout(d time.Duration) {
	c.mu.Lock()
	c.timeout = d
	c.mu.Unlock()
}

// Warning is true when the current mode skips approval entirely.
func (c *Controller) Warning() bool { return c.Mode() == Bypass }

// AllowForSession remembers that this tool is pre-approved for this session.
func (c *Controller) AllowForSession(sessionID, toolName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.allowed[sessionID] == nil {
		c.allowed[sessionID] = map[string]bool{}
	}
	c.allowed[sessionID][toolName] = true
}

// Preapproved reports whether the session already allowed this tool.
func (c *Controller) Preapproved(sessionID, toolName string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.allowed[sessionID][toolName]
}

func (c *Controller) timeoutFor() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.timeout
}

// Approver returns the callback the tool registry consults for one session.
func (c *Controller) Approver(sessionID string) tool.Approver {
	c.mu.RLock()
	ask := c.ask
	c.mu.RUnlock()
	return c.approverFor(sessionID, ask)
}

// ApproverFor binds the callback to a specific asker, so two transports can
// drive the same controller without sharing a callback.
func (c *Controller) ApproverFor(sessionID string, ask Asker) tool.Approver {
	return c.approverFor(sessionID, ask)
}

func (c *Controller) approverFor(sessionID string, ask Asker) tool.Approver {
	return func(ctx context.Context, name string, args []byte) bool {
		mode := c.Mode()
		if mode == Bypass {
			return true
		}
		if c.Preapproved(sessionID, name) {
			return true
		}
		// In Allow mode the first use still asks; the answer is remembered.
		if ask == nil {
			return false
		}

		actx, cancel := context.WithTimeout(ctx, c.timeoutFor())
		defer cancel()
		decision, err := ask(actx, sessionID, name, args)
		if err != nil {
			return false
		}
		switch decision {
		case AllowOnce:
			return true
		case AllowSession:
			c.AllowForSession(sessionID, name)
			return true
		default:
			return false
		}
	}
}

// ErrUnknownMode is returned by SetMode for an unrecognised tier.
var ErrUnknownMode = errorString("perm: unknown mode")

type errorString string

func (e errorString) Error() string { return string(e) }

// NormalizeMode maps user input to a mode, rejecting anything else.
func NormalizeMode(s string) (Mode, error) {
	m := Mode(strings.ToLower(strings.TrimSpace(s)))
	if err := (&Controller{}).SetMode(m); err != nil {
		return "", err
	}
	return m, nil
}
