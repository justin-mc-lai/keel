package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSandboxRejectsEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}

	ws, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}

	// Relative escape.
	if _, err := ws.Resolve("../../etc/passwd"); !errors.Is(err, ErrOutside) {
		t.Fatalf("parent escape: %v", err)
	}
	// Absolute path outside the root.
	if _, err := ws.Resolve(filepath.Join(outside, "secret.txt")); !errors.Is(err, ErrOutside) {
		t.Fatalf("absolute escape: %v", err)
	}
	// Symlink inside the root pointing outside.
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := ws.Resolve("link/secret.txt"); !errors.Is(err, ErrOutside) {
		t.Fatalf("symlink escape: %v", err)
	}
	// A normal path still works.
	inside, err := ws.Resolve("sub/file.txt")
	if err != nil {
		t.Fatalf("inside path rejected: %v", err)
	}
	if filepath.Dir(inside) == outside {
		t.Fatal("resolved into the wrong tree")
	}
}

func TestAgentsMdLoaded(t *testing.T) {
	root := t.TempDir()
	ws, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := ws.Instructions(); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if err := os.WriteFile(filepath.Join(root, InstructionsFile), []byte("  be nice  \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ws.Instructions(); got != "be nice" {
		t.Fatalf("instructions = %q", got)
	}
}
