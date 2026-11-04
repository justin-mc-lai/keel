// Package backup copies the four state files an keel install owns.
//
// One directory, one manifest, no archive format to get wrong. Restore only
// writes files that the manifest names.
package backup

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// FileSpec is one state file.
type FileSpec struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// Manifest describes one backup.
type Manifest struct {
	CreatedAt int64      `json:"created_at"`
	Files     []FileSpec `json:"files"`
}

// StateDir is where keel keeps its data.
func StateDir() string {
	if v := os.Getenv("KEEL_STATE_DIR"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".keel")
}

// files returns the state files that exist right now.
func files(dir string) []string {
	names := []string{"keel.db", "tickets.json", "runs.json", "usage.jsonl", "channel.jsonl", "config.json"}
	var out []string
	for _, n := range names {
		p := filepath.Join(dir, n)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

// Create writes a timestamped backup under root and returns its path.
func Create(root string, srcDir string) (string, error) {
	if srcDir == "" {
		srcDir = StateDir()
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(root, "keel-"+time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return "", err
	}

	m := Manifest{CreatedAt: time.Now().Unix()}
	for _, src := range files(srcDir) {
		name := filepath.Base(src)
		target := filepath.Join(dst, name)
		if err := copyFile(src, target); err != nil {
			return "", fmt.Errorf("backup %s: %w", name, err)
		}
		st, err := os.Stat(target)
		if err != nil {
			return "", err
		}
		m.Files = append(m.Files, FileSpec{Name: name, Path: name, Size: st.Size()})
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dst, "manifest.json"), append(raw, '\n'), 0o600); err != nil {
		return "", err
	}
	return dst, nil
}

// Restore copies a backup back into srcDir. Files the manifest does not name
// are never touched.
func Restore(backupDir, srcDir string) (int, error) {
	if srcDir == "" {
		srcDir = StateDir()
	}
	raw, err := os.ReadFile(filepath.Join(backupDir, "manifest.json"))
	if err != nil {
		return 0, fmt.Errorf("restore: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return 0, fmt.Errorf("restore: bad manifest: %w", err)
	}
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		return 0, err
	}
	for _, f := range m.Files {
		src := filepath.Join(backupDir, f.Path)
		if filepath.Dir(filepath.Clean(src)) != filepath.Clean(backupDir) {
			return 0, fmt.Errorf("restore: refusing path %q", f.Path)
		}
		if err := copyFile(src, filepath.Join(srcDir, f.Name)); err != nil {
			return 0, fmt.Errorf("restore %s: %w", f.Name, err)
		}
	}
	return len(m.Files), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
