package tool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/justin-mc-lai/agentdesk/internal/provider"
)

func TestUnregisteredToolRejected(t *testing.T) {
	r, err := NewDefaultRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Run(context.Background(), provider.ToolCall{Name: "rm_rf", Arguments: `{}`})
	if !errors.Is(err, ErrUnknownTool) {
		t.Fatalf("want ErrUnknownTool, got %v", err)
	}
}

func TestApprovalGateDeniesByDefault(t *testing.T) {
	r := NewRegistry(nil)
	if err := r.Register(Tool{
		Name:             "danger",
		RequiresApproval: true,
		Run:              func(ctx context.Context, args json.RawMessage) (string, error) { return "ran", nil },
	}); err != nil {
		t.Fatal(err)
	}
	_, err := r.Run(context.Background(), provider.ToolCall{Name: "danger", Arguments: `{}`})
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("want ErrDenied, got %v", err)
	}
}

func TestBuiltinEchoRuns(t *testing.T) {
	r, err := NewDefaultRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Run(context.Background(), provider.ToolCall{Name: "echo", Arguments: `{"text":"hi"}`})
	if err != nil {
		t.Fatal(err)
	}
	if got != "hi" {
		t.Fatalf("got %q", got)
	}
}
