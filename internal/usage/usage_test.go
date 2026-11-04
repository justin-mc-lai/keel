package usage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUsageRollup(t *testing.T) {
	l, err := Open(filepath.Join(t.TempDir(), "usage.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	must := func(r Record) {
		t.Helper()
		if err := l.Add(r); err != nil {
			t.Fatal(err)
		}
	}
	must(Record{Provider: "opencode-go", SessionID: "s1", Prompt: 10, Completion: 5, Total: 15, Cost: 0.01})
	must(Record{Provider: "opencode-go", SessionID: "s2", Prompt: 20, Completion: 5, Total: 25, Cost: 0.02})
	must(Record{Provider: "commandcode", SessionID: "s1", Prompt: 7, Completion: 3, Total: 10, Cost: 0.03})

	byProvider, names, err := l.ByProvider()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "commandcode" || names[1] != "opencode-go" {
		t.Fatalf("names = %v", names)
	}
	if got := byProvider["opencode-go"]; got.Calls != 2 || got.Total != 40 || got.Prompt != 30 {
		t.Fatalf("opencode-go totals = %+v", got)
	}
	if got := byProvider["commandcode"]; got.Calls != 1 || got.Cost != 0.03 {
		t.Fatalf("commandcode totals = %+v", got)
	}

	bySession, sessions, err := l.BySession()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %v", sessions)
	}
	if got := bySession["s1"]; got.Calls != 2 || got.Total != 25 {
		t.Fatalf("s1 totals = %+v", got)
	}
}

func TestCorruptLineIsSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "usage.jsonl")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Add(Record{Provider: "p", Total: 1}); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("{not json\n")
	f.Close()
	if err := l.Add(Record{Provider: "p", Total: 2}); err != nil {
		t.Fatal(err)
	}

	totals, _, err := l.ByProvider()
	if err != nil {
		t.Fatal(err)
	}
	if totals["p"].Calls != 2 || totals["p"].Total != 3 {
		t.Fatalf("totals = %+v", totals)
	}
}
