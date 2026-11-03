package desktop

import (
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
