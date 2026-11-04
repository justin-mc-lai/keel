package diff

import (
	"strings"
	"testing"
)

func TestUnified(t *testing.T) {
	if got := Unified("a.txt", "same", "same"); got != "" {
		t.Fatalf("identical input should be empty, got %q", got)
	}

	got := Unified("a.txt", "one\ntwo\nthree", "one\nTWO\nthree")
	for _, want := range []string{"--- a/a.txt", "+++ b/a.txt", "-two", "+TWO", " one", " three"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}

	created := Unified("new.txt", "", "hello")
	if !strings.Contains(created, "+hello") {
		t.Fatalf("create diff = %q", created)
	}
}
