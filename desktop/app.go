// Package desktop is the Wails binding surface. Transport is an in-process
// event channel (runtime.EventsEmit) — not a local HTTP server and not SSE.
// The provider layer stays channel-based so a headless adapter can be added
// later without touching this file.
package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/justin-mc-lai/agentdesk/internal/agent"
	"github.com/justin-mc-lai/agentdesk/internal/config"
	"github.com/justin-mc-lai/agentdesk/internal/perm"
	"github.com/justin-mc-lai/agentdesk/internal/provider"
	"github.com/justin-mc-lai/agentdesk/internal/store"
	"github.com/justin-mc-lai/agentdesk/internal/tool"
)

// Event names carried to the frontend.
const (
	// EventChat carries every stream event.
	EventChat = "chat:event"
	// EventApproval carries an approval request and its resolution.
	EventApproval = "tool:approval"
)

// Emitter forwards an event to the frontend. Tests replace it.
type Emitter func(name string, data ...any)

// PermissionState is what the toolbar renders.
type PermissionState struct {
	Mode    string `json:"mode"`
	Warning bool   `json:"warning"`
	Pending int    `json:"pending"`
}

// ErrNoStore is returned when the session database could not be opened.
var ErrNoStore = errors.New("session store unavailable")

// App is bound to the frontend.
type App struct {
	ctx   context.Context
	reg   *provider.Registry
	store *store.Store
	emit  Emitter
	perm  *perm.Controller

	mu      sync.Mutex
	running map[string]context.CancelFunc
	pending map[string]chan perm.Decision
}

// New builds the app and loads persisted state.
func New() *App {
	reg, err := config.Load()
	if err != nil {
		reg = provider.NewRegistry()
	}
	st, err := store.Open("")
	if err != nil {
		if st, err = store.Open(":memory:"); err != nil {
			log.Printf("agentdesk: session store unavailable: %v", err)
			st = nil
		}
	}
	return NewWith(reg, st)
}

// NewWith wires explicit dependencies (tests and embedders).
func NewWith(reg *provider.Registry, st *store.Store) *App {
	a := &App{
		reg:     reg,
		store:   st,
		running: map[string]context.CancelFunc{},
		pending: map[string]chan perm.Decision{},
	}
	a.perm = perm.New(a.askApproval)
	return a
}

// Startup is called by Wails.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	if a.emit == nil {
		a.emit = func(name string, data ...any) { runtime.EventsEmit(ctx, name, data...) }
	}
}

// Shutdown is called by Wails.
func (a *App) Shutdown(ctx context.Context) {
	if a.store != nil {
		_ = a.store.Close()
	}
}

// SetEmitter overrides the event sink (tests only).
func (a *App) SetEmitter(e Emitter) { a.emit = e }

// Providers returns the configured vendors for the settings panel.
func (a *App) Providers() []provider.Info { return a.reg.List() }

// Sessions lists stored conversations.
func (a *App) Sessions() ([]store.Session, error) {
	if a.store == nil {
		return nil, ErrNoStore
	}
	return a.store.ListSessions()
}

// History returns one conversation's messages.
func (a *App) History(sessionID string) ([]provider.Message, error) {
	if a.store == nil {
		return nil, ErrNoStore
	}
	return a.store.Messages(sessionID)
}

// NewSession creates a conversation and returns its id.
func (a *App) NewSession() (string, error) {
	if a.store == nil {
		return "", ErrNoStore
	}
	return a.store.CreateSession("")
}

// Cancel stops an in-flight turn, including any approval it is waiting on.
func (a *App) Cancel(sessionID string) {
	a.mu.Lock()
	cancel, ok := a.running[sessionID]
	a.mu.Unlock()
	if ok {
		cancel()
	}
	// Session grants deliberately survive a cancel: "本会话允许" means the
	// session, not the turn.
}

// PermissionState returns the current tier and whether it is a warning state.
func (a *App) PermissionState() PermissionState {
	a.mu.Lock()
	pending := len(a.pending)
	a.mu.Unlock()
	return PermissionState{
		Mode:    string(a.perm.Mode()),
		Warning: a.perm.Warning(),
		Pending: pending,
	}
}

// SetPermissionMode switches the tier: ask / allow / bypass.
func (a *App) SetPermissionMode(mode string) (PermissionState, error) {
	m, err := perm.NormalizeMode(mode)
	if err != nil {
		return a.PermissionState(), err
	}
	if err := a.perm.SetMode(m); err != nil {
		return a.PermissionState(), err
	}
	return a.PermissionState(), nil
}

// ResolveApproval answers one pending approval request.
func (a *App) ResolveApproval(id, decision string) error {
	d := perm.Decision(decision)
	switch d {
	case perm.AllowOnce, perm.AllowSession, perm.Deny:
	default:
		return fmt.Errorf("unknown decision %q", decision)
	}
	a.mu.Lock()
	ch, ok := a.pending[id]
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("no pending approval %q", id)
	}
	select {
	case ch <- d:
		return nil
	default:
		return fmt.Errorf("approval %q already answered", id)
	}
}

// Send runs one turn. Tokens arrive on EventChat while it runs; the returned
// string is the final answer so a caller that lost the stream can recover.
func (a *App) Send(sessionID, providerName, model, effort, text string) (string, error) {
	if a.store == nil {
		return "", ErrNoStore
	}
	if sessionID == "" {
		id, err := a.store.CreateSession("")
		if err != nil {
			return "", err
		}
		sessionID = id
	}
	p, err := a.pick(providerName)
	if err != nil {
		return "", err
	}

	tools, err := tool.NewDefaultRegistry(a.perm.Approver(sessionID))
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithCancel(context.Background())
	if a.ctx != nil {
		ctx, cancel = context.WithCancel(a.ctx)
	}
	a.mu.Lock()
	a.running[sessionID] = cancel
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.running, sessionID)
		a.mu.Unlock()
		cancel()
	}()

	loop := agent.New(agent.Options{
		Provider: p,
		Tools:    tools,
		Store:    a.store,
		Model:    model,
		Effort:   effort,
		OnEvent:  func(ev provider.Event) { a.emitEvent(EventChat, ev) },
	})
	return loop.Run(ctx, sessionID, text)
}

// askApproval bridges a blocking tool approval to the frontend. perm bounds
// the wait; ctx.Done also unblocks it when the turn is cancelled.
func (a *App) askApproval(ctx context.Context, sessionID, toolName string, args []byte) (perm.Decision, error) {
	id, err := newID()
	if err != nil {
		return perm.Deny, err
	}
	ch := make(chan perm.Decision, 1)
	a.mu.Lock()
	a.pending[id] = ch
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.pending, id)
		a.mu.Unlock()
	}()

	a.emitEvent(EventApproval, map[string]any{
		"id":         id,
		"sessionId":  sessionID,
		"tool":       toolName,
		"args":       string(args),
		"timeoutSec": int(perm.DefaultTimeout / time.Second),
	})

	select {
	case d := <-ch:
		return d, nil
	case <-ctx.Done():
		a.emitEvent(EventApproval, map[string]any{
			"id": id, "closed": true, "decision": string(perm.Deny),
		})
		return perm.Deny, ctx.Err()
	}
}

func (a *App) pick(name string) (provider.Provider, error) {
	if name == "" {
		p := a.reg.Default()
		if p == nil {
			return nil, errors.New("no provider configured")
		}
		return p, nil
	}
	p, ok := a.reg.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", name)
	}
	return p, nil
}

func (a *App) emitEvent(name string, data ...any) {
	if a.emit != nil {
		a.emit(name, data...)
	}
}

func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ap_" + hex.EncodeToString(b), nil
}
