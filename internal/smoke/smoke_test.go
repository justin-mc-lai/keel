// Package smoke holds the one test that talks to real vendors. It is the
// difference between "the code compiles" and "the keys actually work".
package smoke

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/justin-mc-lai/agentdesk/internal/config"
	"github.com/justin-mc-lai/agentdesk/internal/provider"
)

func TestLiveProviders(t *testing.T) {
	reg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	var available []provider.Info
	for _, info := range reg.List() {
		if info.Available {
			available = append(available, info)
		}
	}
	if len(available) == 0 {
		t.Skip("no provider API keys in environment (set OPENCODE_GO_API_KEY / COMMAND_CODE_API_KEY)")
	}

	for _, info := range available {
		info := info
		t.Run(info.Name, func(t *testing.T) {
			p, ok := reg.Get(info.Name)
			if !ok {
				t.Fatalf("provider %q missing from registry", info.Name)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			stream, err := p.Stream(ctx, provider.Request{
				Messages:  []provider.Message{{Role: provider.RoleUser, Content: "reply with exactly: PONG"}},
				MaxTokens: 128,
				SessionID: "agentdesk-smoke",
			})
			if err != nil {
				t.Fatalf("stream start: %v", err)
			}

			var text strings.Builder
			var sawDone bool
			for ev := range stream {
				switch ev.Type {
				case provider.EventContent:
					text.WriteString(ev.Text)
				case provider.EventError:
					t.Fatalf("stream error: %v", ev.Err)
				case provider.EventDone:
					sawDone = true
				}
			}
			if !sawDone {
				t.Fatal("stream ended without a done event")
			}
			got := strings.TrimSpace(text.String())
			if got == "" {
				t.Fatal("empty assistant content")
			}
			t.Logf("%s -> %q", info.Name, got)
		})
	}
}
