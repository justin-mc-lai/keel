// Package core is the transport-neutral service behind both shells.
//
// It knows nothing about Wails and nothing about HTTP. Each shell supplies an
// EventSink and forwards approval decisions back through ResolveApproval.
package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/justin-mc-lai/keel/internal/agent"
	"github.com/justin-mc-lai/keel/internal/config"
	"github.com/justin-mc-lai/keel/internal/diff"
	"github.com/justin-mc-lai/keel/internal/mcp"
	"github.com/justin-mc-lai/keel/internal/perm"
	"github.com/justin-mc-lai/keel/internal/provider"
	"github.com/justin-mc-lai/keel/internal/store"
	"github.com/justin-mc-lai/keel/internal/tool"
	"github.com/justin-mc-lai/keel/internal/usage"
	"github.com/justin-mc-lai/keel/internal/workspace"
)

// Event names shared by every transport.
const (
	EventChat     = "chat:event"
	EventApproval = "tool:approval"
	EventDiff     = "tool:diff"
)

// EventSink delivers one event to a shell.
type EventSink func(name string, data any)

// ErrNoStore is returned when the session database could not be opened.
var ErrNoStore = errors.New("session store unavailable")

// PermissionState is what a toolbar renders.
type PermissionState struct {
	Mode    string `json:"mode"`
	Warning bool   `json:"warning"`
	Pending int    `json:"pending"`
}

// SendRequest is one turn.
type SendRequest struct {
	SessionID string `json:"sessionId"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
	Text      string `json:"text"`
}

// Service owns every piece of state a shell can touch.
type Service struct {
	reg   *provider.Registry
	store *store.Store
	perm  *perm.Controller

	mu      sync.Mutex
	running map[string]context.CancelFunc
	pending map[string]chan perm.Decision
	mcp     []*mcp.Client
	mcpErrs []error
}

// New builds the service and seeds it with persisted settings.
func New() *Service {
	reg, err := config.Load()
	if err != nil {
		reg = provider.NewRegistry()
	}
	st, err := store.Open("")
	if err != nil {
		if st, err = store.Open(":memory:"); err != nil {
			st = nil
		}
	}
	return NewWith(reg, st)
}

// NewWith wires explicit dependencies (tests and embedders).
func NewWith(reg *provider.Registry, st *store.Store) *Service {
	s := &Service{
		reg:     reg,
		store:   st,
		running: map[string]context.CancelFunc{},
		pending: map[string]chan perm.Decision{},
	}
	s.perm = perm.New(nil)
	return s
}

// Bootstrap applies persisted settings and starts configured MCP servers.
func (s *Service) Bootstrap() {
	s.ApplySettings(config.LoadSettings())
	cfg, err := config.LoadFile()
	if err != nil || len(cfg.MCPServers) == 0 {
		return
	}
	clients, errs := mcp.StartAll(cfg.MCPServers)
	s.mcp = clients
	s.mcpErrs = errs
}

// Close releases long-lived resources.
func (s *Service) Close() {
	for _, c := range s.mcp {
		_ = c.Close()
	}
	if s.store != nil {
		_ = s.store.Close()
	}
}

// Store exposes the session store for tests and shells that need raw access.
func (s *Service) Store() *store.Store { return s.store }

// Registry exposes the provider registry.
func (s *Service) Registry() *provider.Registry { return s.reg }

// ApplySettings pushes persisted defaults into the running objects.
func (s *Service) ApplySettings(cfg config.Settings) {
	if cfg.DefaultProvider != "" {
		s.reg.SetDefault(cfg.DefaultProvider)
	}
	if cfg.DefaultApproval != "" {
		if m, err := perm.NormalizeMode(cfg.DefaultApproval); err == nil {
			_ = s.perm.SetMode(m)
		}
	}
}

// Providers returns the configured vendors.
func (s *Service) Providers() []provider.Info { return s.reg.List() }

// Sessions lists stored conversations.
func (s *Service) Sessions() ([]store.Session, error) {
	if s.store == nil {
		return nil, ErrNoStore
	}
	return s.store.ListSessions()
}

// History returns one conversation's messages.
func (s *Service) History(sessionID string) ([]provider.Message, error) {
	if s.store == nil {
		return nil, ErrNoStore
	}
	return s.store.Messages(sessionID)
}

// NewSession creates a conversation.
func (s *Service) NewSession() (string, error) {
	if s.store == nil {
		return "", ErrNoStore
	}
	return s.store.CreateSession("")
}

// RenameSession sets a title.
func (s *Service) RenameSession(id, title string) error {
	if s.store == nil {
		return ErrNoStore
	}
	return s.store.RenameSession(id, title)
}

// DeleteSession removes a conversation.
func (s *Service) DeleteSession(id string) error {
	if s.store == nil {
		return ErrNoStore
	}
	return s.store.DeleteSession(id)
}

// SearchSessions filters by title.
func (s *Service) SearchSessions(query string) ([]store.Session, error) {
	if s.store == nil {
		return nil, ErrNoStore
	}
	if strings.TrimSpace(query) == "" {
		return s.store.ListSessions()
	}
	return s.store.SearchSessions(query)
}

// ExportSession returns one conversation as JSON text.
func (s *Service) ExportSession(id string) (string, error) {
	if s.store == nil {
		return "", ErrNoStore
	}
	raw, err := s.store.ExportSession(id)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// PermissionState reports the current tier.
func (s *Service) PermissionState() PermissionState {
	s.mu.Lock()
	pending := len(s.pending)
	s.mu.Unlock()
	return PermissionState{Mode: string(s.perm.Mode()), Warning: s.perm.Warning(), Pending: pending}
}

// SetPermissionMode switches the tier.
func (s *Service) SetPermissionMode(mode string) (PermissionState, error) {
	m, err := perm.NormalizeMode(mode)
	if err != nil {
		return s.PermissionState(), err
	}
	if err := s.perm.SetMode(m); err != nil {
		return s.PermissionState(), err
	}
	return s.PermissionState(), nil
}

// ResolveApproval answers one pending request.
func (s *Service) ResolveApproval(id, decision string) error {
	d := perm.Decision(decision)
	switch d {
	case perm.AllowOnce, perm.AllowSession, perm.Deny:
	default:
		return fmt.Errorf("unknown decision %q", decision)
	}
	s.mu.Lock()
	ch, ok := s.pending[id]
	s.mu.Unlock()
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

// Settings returns the persisted settings block.
func (s *Service) Settings() config.Settings { return config.LoadSettings() }

// SaveSettings persists and applies the block.
func (s *Service) SaveSettings(cfg config.Settings) (config.Settings, error) {
	if err := config.SaveSettings(cfg); err != nil {
		return config.LoadSettings(), err
	}
	cur := config.LoadSettings()
	s.ApplySettings(cur)
	return cur, nil
}

// UsageRollup returns token totals by provider and by session.
func (s *Service) UsageRollup() (map[string]usage.Totals, map[string]usage.Totals, error) {
	l, err := usage.Open("")
	if err != nil {
		return nil, nil, err
	}
	byProvider, _, err := l.ByProvider()
	if err != nil {
		return nil, nil, err
	}
	bySession, _, err := l.BySession()
	if err != nil {
		return nil, nil, err
	}
	return byProvider, bySession, nil
}

// WorkspaceRoot resolves the effective workspace directory.
func (s *Service) WorkspaceRoot() string {
	if cfg := config.LoadSettings(); cfg.WorkspaceRoot != "" {
		return cfg.WorkspaceRoot
	}
	if wd, err := os.Getwd(); err == nil && wd != "/" {
		return wd
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}

// MCPStatus reports started servers and failures.
func (s *Service) MCPStatus() map[string]any {
	failed := make([]string, 0, len(s.mcpErrs))
	for _, err := range s.mcpErrs {
		failed = append(failed, err.Error())
	}
	names := make([]string, 0, len(s.mcp))
	for _, c := range s.mcp {
		names = append(names, c.Name())
	}
	return map[string]any{"servers": names, "errors": failed}
}

// Cancel stops an in-flight turn.
func (s *Service) Cancel(sessionID string) {
	s.mu.Lock()
	cancel, ok := s.running[sessionID]
	s.mu.Unlock()
	if ok {
		cancel()
	}
}

// Retry drops the newest user turn and resends it.
func (s *Service) Retry(ctx context.Context, req SendRequest, sink EventSink) (string, error) {
	if s.store == nil {
		return "", ErrNoStore
	}
	id, text, err := s.store.LastUserMessage(req.SessionID)
	if err != nil {
		return "", err
	}
	if err := s.store.TruncateAfter(req.SessionID, id-1); err != nil {
		return "", err
	}
	req.Text = text
	return s.Send(ctx, req, sink)
}

// Send runs one turn, streaming to sink.
func (s *Service) Send(ctx context.Context, req SendRequest, sink EventSink) (string, error) {
	if s.store == nil {
		return "", ErrNoStore
	}
	sessionID := req.SessionID
	if sessionID == "" {
		id, err := s.store.CreateSession("")
		if err != nil {
			return "", err
		}
		sessionID = id
	}
	p, err := s.pick(req.Provider)
	if err != nil {
		return "", err
	}
	ws, err := workspace.Open(s.WorkspaceRoot())
	if err != nil {
		return "", err
	}

	ask := s.approverFor(sessionID, sink)
	tools, err := tool.NewDefaultRegistry(ask, ws)
	if err != nil {
		return "", err
	}
	for _, e := range mcp.RegisterTools(tools, s.mcp) {
		sink(EventChat, provider.Event{Type: provider.EventError, Err: e})
	}
	tools.SetPreview(func(pv tool.Preview) error {
		sink(EventDiff, map[string]any{"path": pv.Path, "diff": diff.Unified(pv.Path, pv.Before, pv.After)})
		return nil
	})

	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.running[sessionID] = cancel
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.running, sessionID)
		s.mu.Unlock()
		cancel()
	}()

	loop := agent.New(agent.Options{
		Provider:     p,
		Tools:        tools,
		Store:        s.store,
		Model:        req.Model,
		Effort:       req.Effort,
		Instructions: ws.Instructions(),
		OnEvent: func(ev provider.Event) {
			if ev.Type == provider.EventUsage && ev.Usage != nil {
				if l, err := usage.Open(""); err == nil {
					_ = l.Add(usage.Record{
						Provider: p.Name(), Model: req.Model, SessionID: sessionID,
						Prompt: ev.Usage.PromptTokens, Completion: ev.Usage.CompletionTokens,
						Total: ev.Usage.TotalTokens,
					})
				}
			}
			sink(EventChat, ev)
		},
	})
	if err := tools.SetSubagent(loop.Subagent()); err != nil {
		return "", err
	}
	text := agent.ExpandAttachments(ws, req.Text)
	return loop.Run(runCtx, sessionID, text)
}

// approverFor builds the approval callback bound to one stream.
func (s *Service) approverFor(sessionID string, sink EventSink) tool.Approver {
	ask := func(ctx context.Context, sid, toolName string, args []byte) (perm.Decision, error) {
		id, err := newID()
		if err != nil {
			return perm.Deny, err
		}
		ch := make(chan perm.Decision, 1)
		s.mu.Lock()
		s.pending[id] = ch
		s.mu.Unlock()
		defer func() {
			s.mu.Lock()
			delete(s.pending, id)
			s.mu.Unlock()
		}()
		sink(EventApproval, map[string]any{
			"id": id, "sessionId": sid, "tool": toolName,
			"args": string(args), "timeoutSec": int(perm.DefaultTimeout.Seconds()),
		})
		select {
		case d := <-ch:
			return d, nil
		case <-ctx.Done():
			sink(EventApproval, map[string]any{"id": id, "closed": true, "decision": string(perm.Deny)})
			return perm.Deny, ctx.Err()
		}
	}
	return s.perm.ApproverFor(sessionID, ask)
}

func (s *Service) pick(name string) (provider.Provider, error) {
	if name == "" {
		p := s.reg.Default()
		if p == nil {
			return nil, errors.New("no provider configured")
		}
		return p, nil
	}
	p, ok := s.reg.Get(name)
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", name)
	}
	return p, nil
}

func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ap_" + hex.EncodeToString(b), nil
}
