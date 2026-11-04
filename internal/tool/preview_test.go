package tool

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin-mc-lai/keel/internal/diff"
	"github.com/justin-mc-lai/keel/internal/provider"
	"github.com/justin-mc-lai/keel/internal/workspace"
)

func TestDiffPreviewBeforeWrite(t *testing.T) {
	root := t.TempDir()
	ws, err := workspace.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewDefaultRegistry(func(ctx context.Context, name string, args []byte) bool { return true }, ws)
	if err != nil {
		t.Fatal(err)
	}

	var seen []string
	r.SetPreview(func(p Preview) error {
		seen = append(seen, diff.Unified(p.Path, p.Before, p.After))
		return nil
	})

	if _, err := r.Run(context.Background(), provider.ToolCall{
		Name: "write_file", Arguments: `{"path":"a.txt","content":"one\ntwo"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || !strings.Contains(seen[0], "+one") {
		t.Fatalf("preview not emitted for create: %v", seen)
	}

	if _, err := r.Run(context.Background(), provider.ToolCall{
		Name: "edit_file", Arguments: `{"path":"a.txt","old":"two","new":"three"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || !strings.Contains(seen[1], "-two") || !strings.Contains(seen[1], "+three") {
		t.Fatalf("preview not emitted for edit: %v", seen)
	}

	// A rejecting previewer must leave the file untouched.
	r.SetPreview(func(p Preview) error { return errors.New("rejected") })
	if _, err := r.Run(context.Background(), provider.ToolCall{
		Name: "write_file", Arguments: `{"path":"a.txt","content":"clobbered"}`,
	}); err == nil {
		t.Fatal("rejected preview must fail the write")
	}
	raw, err := os.ReadFile(filepath.Join(root, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "one\nthree" {
		t.Fatalf("file was modified despite rejection: %q", raw)
	}
}
