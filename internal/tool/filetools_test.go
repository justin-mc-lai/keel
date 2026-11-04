package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin-mc-lai/keel/internal/provider"
	"github.com/justin-mc-lai/keel/internal/workspace"
)

func TestFileToolsRoundTrip(t *testing.T) {
	root := t.TempDir()
	ws, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewDefaultRegistry(func(ctx context.Context, name string, args []byte) bool { return true }, ws)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"read_file", "write_file", "edit_file", "list_dir", "grep", "fetch_url"}
	have := map[string]bool{}
	for _, spec := range r.Specs() {
		have[spec.Name] = true
		if len(spec.Parameters) == 0 {
			t.Errorf("%s has no JSON schema", spec.Name)
		}
	}
	for _, n := range want {
		if !have[n] {
			t.Errorf("missing tool %s", n)
		}
	}

	call := func(name, args string) string {
		t.Helper()
		out, err := r.Run(context.Background(), provider.ToolCall{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return out
	}

	call("write_file", `{"path":"notes/a.txt","content":"hello world"}`)
	if got := call("read_file", `{"path":"notes/a.txt"}`); got != "hello world" {
		t.Fatalf("read = %q", got)
	}
	call("edit_file", `{"path":"notes/a.txt","old":"world","new":"there"}`)
	if got := call("read_file", `{"path":"notes/a.txt"}`); got != "hello there" {
		t.Fatalf("after edit = %q", got)
	}
	if got := call("list_dir", `{"path":"notes"}`); !strings.Contains(got, "a.txt") {
		t.Fatalf("list = %q", got)
	}
	if got := call("grep", `{"pattern":"there","path":"notes"}`); !strings.Contains(got, "a.txt:1:") {
		t.Fatalf("grep = %q", got)
	}

	// The sandbox must hold for tool calls too.
	if _, err := r.Run(context.Background(), provider.ToolCall{
		Name: "read_file", Arguments: `{"path":"../../etc/passwd"}`,
	}); err == nil {
		t.Fatal("read_file escaped the workspace")
	}

	// edit_file refuses an ambiguous needle.
	call("write_file", `{"path":"dup.txt","content":"x x"}`)
	if _, err := r.Run(context.Background(), provider.ToolCall{
		Name: "edit_file", Arguments: `{"path":"dup.txt","old":"x","new":"y"}`,
	}); err == nil {
		t.Fatal("edit_file should refuse an ambiguous match")
	}

	if _, err := os.Stat(filepath.Join(root, "notes", "a.txt")); err != nil {
		t.Fatal(err)
	}
}
