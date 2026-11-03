// Package desktop is the Wails binding surface. Transport is an in-process
// event channel (runtime.EventsEmit) — not a local HTTP server and not SSE.
// The provider layer stays channel-based so a headless adapter can be added
// later without touching this file.
package desktop

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/justin-mc-lai/agentdesk/internal/agent"
	"github.com/justin-mc-lai/agentdesk/internal/config"
	"github.com/justin-mc-lai/agentdesk/internal/provider"
	"github.com/justin-mc-lai/agentdesk/internal/store"
	"github.com/justin-mc-lai/agentdesk/internal/tool"
)

// EventChat is the single frontend event name carrying every stream event.
const EventChat = "chat:event"

// Emitter forwards an event to the frontend. Tests replace it.
type Emitter func(name string, data ...any)

// App is bound to the frontend.
type App struct {
	ctx   context.Context
	reg   *provider.Registry
	store *store.Store
	emit  Emitter

	mu      sync.Mutex
	running map[string]context.CancelFunc
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
	return &App{reg: reg, store: st, running: map[string]context.CancelFunc{}}
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

// ErrNoStore is returned when the session database could not be opened.
var ErrNoStore = errors.New("session store unavailable")

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

// Cancel stops an in-flight turn.
func (a *App) Cancel(sessionID string) {
	a.mu.Lock()
	cancel, ok := a.running[sessionID]
	a.mu.Unlock()
	if ok {
		cancel()
	}
}

// Send runs one turn. Tokens arrive on EventChat while it runs; the returned
// string is the final answer so a caller that lost the stream can recover.
func (a *App) Send(sessionID, providerName, model, text string) (string, error) {
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

	// Default deny: the desktop has no approval prompt yet, so a tool that
	// asks for approval must not run unattended. Built-in tools do not ask.
	tools, err := tool.NewDefaultRegistry(func(name string, args []byte) bool { return false })
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithCancel(a.ctx)
	if ctx == nil {
		ctx, cancel = context.WithCancel(context.Background())
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
		OnEvent:  func(ev provider.Event) { a.emit(EventChat, ev) },
	})
	return loop.Run(ctx, sessionID, text)
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
