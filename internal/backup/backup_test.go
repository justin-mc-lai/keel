package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupRestoreRoundTrip(t *testing.T) {
	src := t.TempDir()
	t.Setenv("KEEL_STATE_DIR", src)

	want := map[string]string{
		"keel.db":  "db-bytes",
		"tickets.json":  `[{"id":"t1"}]`,
		"usage.jsonl":   `{"total":5}`,
		"channel.jsonl": `{"ticket_id":"t1"}`,
	}
	for name, body := range want {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	backupRoot := t.TempDir()
	dir, err := Create(backupRoot, src)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}

	// Wipe the originals.
	for name := range want {
		if err := os.Remove(filepath.Join(src, name)); err != nil {
			t.Fatal(err)
		}
	}
	// A file that the backup never saw must survive a restore.
	extra := filepath.Join(src, "keep-me.txt")
	if err := os.WriteFile(extra, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}

	n, err := Restore(dir, src)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if n != len(want) {
		t.Fatalf("restored %d files, want %d", n, len(want))
	}
	for name, body := range want {
		got, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(got) != body {
			t.Fatalf("%s = %q, want %q", name, got, body)
		}
	}
	if got, _ := os.ReadFile(extra); string(got) != "untouched" {
		t.Fatalf("restore touched an unrelated file: %q", got)
	}
}

func TestRestoreRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	bad := `{"created_at":0,"files":[{"name":"x","path":"../../etc/passwd","size":1}]}`
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(dir, t.TempDir()); err == nil {
		t.Fatal("path traversal must be refused")
	}
}
