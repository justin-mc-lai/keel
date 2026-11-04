package httpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/justin-mc-lai/agentdesk/internal/core"
	"github.com/justin-mc-lai/agentdesk/internal/provider"
	"github.com/justin-mc-lai/agentdesk/internal/store"
)

type fakeProvider struct{}

func (fakeProvider) Name() string         { return "fake" }
func (fakeProvider) DefaultModel() string { return "fake-1" }
func (fakeProvider) Models() []string     { return []string{"fake-1"} }
func (fakeProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	ch := make(chan provider.Event, 4)
	go func() {
		defer close(ch)
		ch <- provider.Event{Type: provider.EventContent, Text: "PONG"}
		ch <- provider.Event{Type: provider.EventDone}
	}()
	return ch, nil
}

func newServer(t *testing.T) (*Server, *core.Service) {
	t.Helper()
	reg := provider.NewRegistry()
	reg.Add(fakeProvider{})
	st, err := store.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := core.NewWith(reg, st)
	return New(svc, nil), svc
}

func TestHealthz(t *testing.T) {
	srv, _ := newServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("not json: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %v", body)
	}
}

func TestStaticServesFrontend(t *testing.T) {
	// No assets: a clear error, never a blank page.
	srv, _ := newServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "frontend not built") {
		t.Fatalf("missing assets: %d %s", rec.Code, rec.Body.String())
	}

	// With assets: index.html is served, and unknown routes fall back to it.
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>agentdesk</html>")}}
	srv2 := New(srv.svc, assets)
	for _, path := range []string{"/", "/some/spa/route"} {
		rec := httptest.NewRecorder()
		srv2.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "agentdesk") {
			t.Fatalf("%s -> %d %q", path, rec.Code, rec.Body.String())
		}
	}
}

func TestChatStreamsOverSSE(t *testing.T) {
	srv, svc := newServer(t)
	id, _ := svc.NewSession()

	body := `{"sessionId":"` + id + `","provider":"fake","text":"hi"}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(body))
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content type = %q", ct)
	}
	out := rec.Body.String()
	if !strings.Contains(out, `"event":"chat:event"`) {
		t.Fatalf("no chat event in:\n%s", out)
	}
	if !strings.Contains(out, "PONG") {
		t.Fatalf("no content in:\n%s", out)
	}
	if !strings.Contains(out, "chat:final") {
		t.Fatalf("no final event in:\n%s", out)
	}
}

// gatedProvider asks for a tool that requires approval, once.
type gatedProvider struct{ asked bool }

func (g *gatedProvider) Name() string         { return "gated" }
func (g *gatedProvider) DefaultModel() string { return "g" }
func (g *gatedProvider) Models() []string     { return []string{"g"} }
func (g *gatedProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	ch := make(chan provider.Event, 4)
	go func() {
		defer close(ch)
		if !g.asked {
			g.asked = true
			ch <- provider.Event{Type: provider.EventToolCall, ToolCall: &provider.ToolCall{
				ID: "c1", Name: "run_shell", Arguments: `{"command":"echo hi"}`,
			}}
			ch <- provider.Event{Type: provider.EventDone}
			return
		}
		ch <- provider.Event{Type: provider.EventContent, Text: "resumed"}
		ch <- provider.Event{Type: provider.EventDone}
	}()
	return ch, nil
}

func TestApprovalRoundTripOverHTTP(t *testing.T) {
	reg := provider.NewRegistry()
	gp := &gatedProvider{}
	reg.Add(gp)
	st, err := store.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := core.NewWith(reg, st)
	srv := New(svc, nil)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	id, _ := svc.NewSession()
	body := `{"sessionId":"` + id + `","provider":"gated","text":"go"}`
	resp, err := http.Post(ts.URL+"/api/chat", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)

	var approvalID string
	deadline := time.After(5 * time.Second)
	done := make(chan string, 1)

	go func() {
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var ev struct {
				Event string          `json:"event"`
				Data  json.RawMessage `json:"data"`
			}
			if json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev) != nil {
				continue
			}
			if ev.Event == "tool:approval" {
				var a struct {
					ID     string `json:"id"`
					Tool   string `json:"tool"`
					Closed bool   `json:"closed"`
				}
				if json.Unmarshal(ev.Data, &a) == nil && a.ID != "" && !a.Closed {
					select {
					case done <- a.ID:
					default:
					}
				}
			}
		}
		close(done)
	}()

	select {
	case got, ok := <-done:
		if !ok {
			t.Fatal("stream ended before any approval")
		}
		approvalID = got
	case <-deadline:
		t.Fatal("no approval event arrived")
	}
	if approvalID == "" {
		t.Fatal("empty approval id")
	}

	dec := `{"decision":"deny"}`
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/approvals/"+approvalID, strings.NewReader(dec))
	req.Header.Set("Content-Type", "application/json")
	ar, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, ar.Body)
	ar.Body.Close()
	if ar.StatusCode != http.StatusNoContent {
		t.Fatalf("approval status = %d", ar.StatusCode)
	}

	// The turn must continue after the decision, not hang.
	wait := time.After(10 * time.Second)
	for {
		select {
		case _, ok := <-done:
			if !ok {
				return // stream closed: the turn completed
			}
		case <-wait:
			t.Fatal("turn did not resume after the approval decision")
		}
	}
}
