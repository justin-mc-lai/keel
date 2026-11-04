// Package desktop is the Wails adapter over internal/core.
//
// Everything here is transport: bind methods the frontend can call, and turn
// core events into runtime.EventsEmit calls. Business logic lives in core so
// the HTTP adapter can serve the same frontend.
package desktop

import (
	"context"
	"os"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/justin-mc-lai/keel/internal/config"
	"github.com/justin-mc-lai/keel/internal/core"
	"github.com/justin-mc-lai/keel/internal/panel"
	"github.com/justin-mc-lai/keel/internal/provider"
	"github.com/justin-mc-lai/keel/internal/store"
	"github.com/justin-mc-lai/keel/internal/ticket"
	"github.com/justin-mc-lai/keel/internal/usage"
)

// Event names, re-exported for the frontend bindings and tests.
const (
	EventChat     = core.EventChat
	EventApproval = core.EventApproval
	EventDiff     = core.EventDiff
)

// Emitter forwards an event to the frontend. Tests replace it.
type Emitter func(name string, data ...any)

// PermissionState mirrors core's shape for the bindings.
type PermissionState = core.PermissionState

// ErrNoStore mirrors core's error.
var ErrNoStore = core.ErrNoStore

// App is bound to the frontend.
type App struct {
	ctx  context.Context
	svc  *core.Service
	emit Emitter
	mu   sync.Mutex
}

// New builds the app with persisted state.
func New() *App { return &App{svc: core.New()} }

// NewWith wires explicit dependencies (tests and embedders).
func NewWith(reg *provider.Registry, st *store.Store) *App {
	return &App{svc: core.NewWith(reg, st)}
}

// Service exposes the core for tests that need raw access.
func (a *App) Service() *core.Service { return a.svc }

// Startup is called by Wails.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.mu.Lock()
	if a.emit == nil {
		a.emit = func(name string, data ...any) { runtime.EventsEmit(ctx, name, data...) }
	}
	a.mu.Unlock()
	a.svc.Bootstrap()
}

// Shutdown is called by Wails.
func (a *App) Shutdown(ctx context.Context) { a.svc.Close() }

// SetEmitter overrides the event sink (tests only).
func (a *App) SetEmitter(e Emitter) {
	a.mu.Lock()
	a.emit = e
	a.mu.Unlock()
}

func (a *App) sink() core.EventSink {
	a.mu.Lock()
	emit := a.emit
	a.mu.Unlock()
	if emit == nil {
		return func(string, any) {}
	}
	return func(name string, data any) { emit(name, data) }
}

func (a *App) base() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

// Providers returns the configured vendors.
func (a *App) Providers() []provider.Info { return a.svc.Providers() }

// Sessions lists stored conversations.
func (a *App) Sessions() ([]store.Session, error) { return a.svc.Sessions() }

// History returns one conversation's messages.
func (a *App) History(sessionID string) ([]provider.Message, error) {
	return a.svc.History(sessionID)
}

// NewSession creates a conversation.
func (a *App) NewSession() (string, error) { return a.svc.NewSession() }

// RenameSession sets a title.
func (a *App) RenameSession(id, title string) error { return a.svc.RenameSession(id, title) }

// DeleteSession removes a conversation.
func (a *App) DeleteSession(id string) error { return a.svc.DeleteSession(id) }

// SearchSessions filters by title.
func (a *App) SearchSessions(q string) ([]store.Session, error) { return a.svc.SearchSessions(q) }

// ExportSession returns one conversation as JSON text.
func (a *App) ExportSession(id string) (string, error) { return a.svc.ExportSession(id) }

// Cancel stops an in-flight turn.
func (a *App) Cancel(sessionID string) { a.svc.Cancel(sessionID) }

// PermissionState reports the current tier.
func (a *App) PermissionState() PermissionState { return a.svc.PermissionState() }

// SetPermissionMode switches the tier.
func (a *App) SetPermissionMode(mode string) (PermissionState, error) {
	return a.svc.SetPermissionMode(mode)
}

// ResolveApproval answers one pending request.
func (a *App) ResolveApproval(id, decision string) error { return a.svc.ResolveApproval(id, decision) }

// Settings returns the persisted settings block.
func (a *App) Settings() config.Settings { return a.svc.Settings() }

// SaveSettings persists and applies the block.
func (a *App) SaveSettings(s config.Settings) (config.Settings, error) { return a.svc.SaveSettings(s) }

// UsageRollup returns token totals.
func (a *App) UsageRollup() (map[string]usage.Totals, map[string]usage.Totals, error) {
	return a.svc.UsageRollup()
}

// WorkspaceRoot returns the effective workspace directory.
func (a *App) WorkspaceRoot() string { return a.svc.WorkspaceRoot() }

// MCPStatus reports started servers and failures.
func (a *App) MCPStatus() map[string]any { return a.svc.MCPStatus() }

// Tickets returns the board data.
func (a *App) Tickets() ([]ticket.Ticket, error) { return a.svc.Tickets() }

// PanelSnapshot answers the three supervisor questions.
func (a *App) PanelSnapshot(historyFor string) (panel.Snapshot, error) {
	return a.svc.PanelSnapshot(historyFor)
}

// ApplyTicketAction advances one ticket.
func (a *App) ApplyTicketAction(id, action string) (ticket.Ticket, error) {
	return a.svc.ApplyTicketAction(id, action)
}

// ExportSessionToFile asks for a path and writes the conversation there.
// Browsers download; a webview cannot, so the desktop gets a real save dialog.
func (a *App) ExportSessionToFile(id string) (string, error) {
	raw, err := a.svc.ExportSession(id)
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(a.base(), runtime.SaveDialogOptions{
		Title:           "导出会话",
		DefaultFilename: "keel-" + id + ".json",
		Filters:         []runtime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // user cancelled
	}
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// Send runs one turn and streams through the Wails event channel.
func (a *App) Send(sessionID, providerName, model, effort, text string) (string, error) {
	return a.svc.Send(a.base(), core.SendRequest{
		SessionID: sessionID, Provider: providerName, Model: model,
		Effort: effort, Text: text,
	}, a.sink())
}

// Retry drops the newest turn and resends it.
func (a *App) Retry(sessionID, providerName, model, effort string) (string, error) {
	return a.svc.Retry(a.base(), core.SendRequest{
		SessionID: sessionID, Provider: providerName, Model: model, Effort: effort,
	}, a.sink())
}

func (a *App) emitEvent(name string, data ...any) { a.sink()(name, data) }
