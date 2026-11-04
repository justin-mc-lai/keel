package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	appcfg "github.com/justin-mc-lai/keel/internal/config"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func TestOpenSourceFilesPresent(t *testing.T) {
	root := repoRoot(t)

	lic, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		t.Fatalf("LICENSE: %v", err)
	}
	if !strings.Contains(string(lic), "MIT License") {
		t.Fatal("LICENSE is not MIT")
	}

	if _, err := os.Stat(filepath.Join(root, "README.md")); err != nil {
		t.Fatalf("README.md: %v", err)
	}

	gi, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf(".gitignore: %v", err)
	}
	for _, want := range []string{".env", "*.db"} {
		if !strings.Contains(string(gi), want) {
			t.Errorf(".gitignore missing %q", want)
		}
	}
}

func TestSecretsNeverCommitted(t *testing.T) {
	root := repoRoot(t)
	// Split so this file does not match its own patterns.
	pats := []*regexp.Regexp{
		regexp.MustCompile(`oc` + `_sk_[A-Za-z0-9]{16,}`),
		regexp.MustCompile(`user` + `_[A-Za-z0-9]{40,}`),
		regexp.MustCompile(`sk` + `-[A-Za-z0-9]{40,}`),
	}

	skipDirs := map[string]bool{
		".git": true, "node_modules": true, "dist": true, "bin": true, ".beacon": true,
	}
	skipFiles := map[string]bool{".env": true, ".env.local": true}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		base := info.Name()
		if skipFiles[base] || strings.HasSuffix(base, ".db") || strings.HasSuffix(base, ".png") ||
			strings.HasSuffix(base, ".ico") || strings.HasSuffix(base, ".woff2") {
			return nil
		}
		if info.Size() > 2<<20 {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil || !utf8.Valid(raw) {
			return nil
		}
		text := string(raw)
		for _, p := range pats {
			if p.MatchString(text) {
				t.Errorf("possible secret in %s (%s)", path, p.String())
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSettingsPersist(t *testing.T) {
	_ = appcfg.Settings{}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("KEEL_CONFIG", path)

	in := appcfg.Settings{
		DefaultProvider: "commandcode",
		DefaultApproval: "allow",
		WorkspaceRoot:   "/tmp/project",
	}
	if err := appcfg.SaveSettings(in); err != nil {
		t.Fatalf("save: %v", err)
	}
	got := appcfg.LoadSettings()
	if got != in {
		t.Fatalf("round trip = %+v, want %+v", got, in)
	}

	// Saving again must not drop provider entries written by other tools.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc["providers"] = []any{map[string]any{"name": "keep-me"}}
	merged, _ := json.Marshal(doc)
	if err := os.WriteFile(path, merged, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := appcfg.SaveSettings(appcfg.Settings{DefaultApproval: "bypass"}); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(path)
	if !strings.Contains(string(raw), "keep-me") {
		t.Fatalf("provider entry lost: %s", raw)
	}
	if appcfg.LoadSettings().DefaultApproval != "bypass" {
		t.Fatal("approval not merged")
	}
}
