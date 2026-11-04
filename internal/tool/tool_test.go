package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/justin-mc-lai/keel/internal/provider"
)

func TestUnregisteredToolRejected(t *testing.T) {
	r, err := NewDefaultRegistry(nil, nil)
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
	r, err := NewDefaultRegistry(nil, nil)
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

func TestShellRequiresApproval(t *testing.T) {
	r, err := NewDefaultRegistry(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Run(context.Background(), provider.ToolCall{
		Name: "run_shell", Arguments: `{"command":"echo hi"}`,
	})
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("run_shell must be denied without an approver, got %v", err)
	}

	r2, err := NewDefaultRegistry(func(ctx context.Context, name string, args []byte) bool { return true }, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r2.Run(context.Background(), provider.ToolCall{
		Name: "run_shell", Arguments: `{"command":"echo hi"}`,
	})
	if err != nil {
		t.Fatalf("approved run_shell: %v", err)
	}
	if !strings.Contains(out, "hi") {
		t.Fatalf("output = %q", out)
	}
}

func TestShellOutputIsCapped(t *testing.T) {
	r, err := NewDefaultRegistry(func(ctx context.Context, name string, args []byte) bool { return true }, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Run(context.Background(), provider.ToolCall{
		Name: "run_shell", Arguments: `{"command":"yes truncated | head -c 40000"}`,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(out) > 8300 {
		t.Fatalf("output not capped: %d bytes", len(out))
	}
	if !strings.Contains(out, "output truncated") {
		t.Fatal("expected truncation marker")
	}
}

func TestShellRespectsContextDeadline(t *testing.T) {
	r, err := NewDefaultRegistry(func(ctx context.Context, name string, args []byte) bool { return true }, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	// The grandchild (`sleep 5`) inherits the pipe. Without a process-group
	// kill this returns after the child finishes, not at the deadline.
	_, _ = r.Run(ctx, provider.ToolCall{Name: "run_shell", Arguments: `{"command":"sleep 5; echo x"}`})
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("command outlived its deadline: %v", elapsed)
	}
}
