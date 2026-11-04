// Package httpserver is the browser adapter over internal/core.
//
// Same service as the desktop shell; the only difference is how events leave
// the process: SSE here, runtime.EventsEmit there.
package httpserver

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/justin-mc-lai/keel/internal/config"
	"github.com/justin-mc-lai/keel/internal/core"
	"github.com/justin-mc-lai/keel/internal/obs"
	"github.com/justin-mc-lai/keel/internal/ticket"
)

// Server serves the API and the built frontend.
type Server struct {
	svc    *core.Service
	assets fs.FS
}

// New builds a server. assets may be nil, in which case / reports a clear
// error instead of serving nothing.
func New(svc *core.Service, assets fs.FS) *Server {
	return &Server{svc: svc, assets: assets}
}

// Handler returns the routed mux.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /metrics", s.metrics)
	mux.HandleFunc("GET /api/providers", s.providers)
	mux.HandleFunc("GET /api/sessions", s.listSessions)
	mux.HandleFunc("POST /api/sessions", s.newSession)
	mux.HandleFunc("PATCH /api/sessions/{id}", s.renameSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.deleteSession)
	mux.HandleFunc("GET /api/sessions/{id}/messages", s.history)
	mux.HandleFunc("GET /api/sessions/{id}/export", s.exportSession)
	mux.HandleFunc("GET /api/permission", s.getPermission)
	mux.HandleFunc("POST /api/permission", s.setPermission)
	mux.HandleFunc("POST /api/approvals/{id}", s.resolveApproval)
	mux.HandleFunc("GET /api/settings", s.getSettings)
	mux.HandleFunc("POST /api/settings", s.saveSettings)
	mux.HandleFunc("GET /api/usage", s.usage)
	mux.HandleFunc("GET /api/tickets", s.tickets)
	mux.HandleFunc("POST /api/tickets/{id}/actions", s.ticketAction)
	mux.HandleFunc("GET /api/panel", s.panel)
	mux.HandleFunc("POST /api/chat", s.chat)
	mux.HandleFunc("GET /", s.static)

	return countRequests(mux)
}

// RequestName is the Prometheus counter for HTTP requests.
const RequestName = "keel_http_requests_total"

func countRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		obs.Inc(RequestName)
		next.ServeHTTP(w, r)
	})
}

// metrics renders counters and live gauges.
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	if list, err := s.svc.Tickets(); err == nil {
		byState := map[ticket.State]int{}
		for _, t := range list {
			byState[t.State]++
		}
		for _, st := range []ticket.State{ticket.Open, ticket.Queued, ticket.Working, ticket.AwaitingPerson, ticket.AwaitingReview, ticket.Failed, ticket.Ended} {
			state := st
			obs.Set("keel_tickets_"+string(state), func() int64 { return int64(byState[state]) })
		}
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = io.WriteString(w, obs.Render())
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().Unix()})
}

func (s *Server) providers(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.svc.Providers())
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.SearchSessions(r.URL.Query().Get("q"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) newSession(w http.ResponseWriter, r *http.Request) {
	id, err := s.svc.NewSession()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) renameSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	if err := s.svc.RenameSession(r.PathValue("id"), body.Title); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.DeleteSession(r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	msgs, err := s.svc.History(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (s *Server) exportSession(w http.ResponseWriter, r *http.Request) {
	raw, err := s.svc.ExportSession(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, raw)
}

func (s *Server) getPermission(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.svc.PermissionState())
}

func (s *Server) setPermission(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	state, err := s.svc.SetPermissionMode(body.Mode)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (s *Server) resolveApproval(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Decision string `json:"decision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	if err := s.svc.ResolveApproval(r.PathValue("id"), body.Decision); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.svc.Settings())
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	var cfg config.Settings
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	saved, err := s.svc.SaveSettings(cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) usage(w http.ResponseWriter, r *http.Request) {
	byProvider, bySession, err := s.svc.UsageRollup()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"byProvider": byProvider, "bySession": bySession})
}

func (s *Server) tickets(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.Tickets()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) ticketAction(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	t, err := s.svc.ApplyTicketAction(r.PathValue("id"), body.Action)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "state": string(t.State)})
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) panel(w http.ResponseWriter, r *http.Request) {
	snap, err := s.svc.PanelSnapshot(r.URL.Query().Get("ticket"))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// chat streams one turn as Server-Sent Events.
func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	var req core.SendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad json"})
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	send := func(name string, data any) {
		payload, err := json.Marshal(map[string]any{"event": name, "data": data})
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
	}

	answer, err := s.svc.Send(r.Context(), req, send)
	send("chat:final", map[string]any{"answer": answer, "error": errString(err)})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// static serves the built frontend, falling back to index.html for SPA routes.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if s.assets == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "frontend not built: run `cd frontend && npm run build`",
		})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if _, err := fs.Stat(s.assets, path); err != nil {
		path = "index.html"
	}
	data, err := fs.ReadFile(s.assets, path)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "frontend not built: run `cd frontend && npm run build`",
		})
		return
	}
	http.ServeContent(w, r, path, time.Time{}, strings.NewReader(string(data)))
}
