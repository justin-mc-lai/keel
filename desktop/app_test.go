package desktop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/justin-mc-lai/agentdesk/internal/provider"
	"github.com/justin-mc-lai/agentdesk/internal/store"
)

func TestStreamEventsWired(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	app := NewWith(provider.NewRegistry(), st)

	var name string
	var payload provider.Event
	app.SetEmitter(func(n string, data ...any) {
		name = n
		if len(data) > 0 {
			payload = data[0].(provider.Event)
		}
	})

	app.emit(EventChat, provider.Event{Type: provider.EventContent, Text: "tok"})

	if name != EventChat {
		t.Fatalf("event name = %q, want %q", name, EventChat)
	}
	if payload.Type != provider.EventContent || payload.Text != "tok" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestWailsConfigPointsAtSingleEntry(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "wails.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Name           string `json:"name"`
		OutputFilename string `json:"outputfilename"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "agentdesk" || cfg.OutputFilename != "agentdesk" {
		t.Fatalf("wails.json = %+v", cfg)
	}
}

func TestBypassWarningVisible(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	app := NewWith(provider.NewRegistry(), st)

	state := app.PermissionState()
	if state.Mode != "ask" || state.Warning {
		t.Fatalf("default state = %+v", state)
	}

	state, err = app.SetPermissionMode("bypass")
	if err != nil {
		t.Fatalf("set bypass: %v", err)
	}
	if state.Mode != "bypass" || !state.Warning {
		t.Fatalf("bypass state = %+v", state)
	}

	if _, err := app.SetPermissionMode("yolo"); err == nil {
		t.Fatal("unknown mode should be rejected")
	}

	// Bypass must allow without emitting an approval request.
	var events int
	app.SetEmitter(func(name string, data ...any) { events++ })
	if !app.perm.Approver("s1")(context.Background(), "danger", nil) {
		t.Fatal("bypass should allow the tool")
	}
	if events != 0 {
		t.Fatalf("bypass emitted %d events, want 0", events)
	}
}

func TestResolveApprovalUnknownID(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	app := NewWith(provider.NewRegistry(), st)
	if err := app.ResolveApproval("ap_missing", "allow_once"); err == nil {
		t.Fatal("expected error for unknown approval id")
	}
	if err := app.ResolveApproval("ap_missing", "maybe"); err == nil {
		t.Fatal("expected error for unknown decision")
	}
}
