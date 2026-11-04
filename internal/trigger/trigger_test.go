package trigger

import (
	"path/filepath"
	"testing"
	"time"
)

func TestScheduleFiresOncePerInstant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "triggers.jsonl")
	led, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	sc := Schedule{ID: "daily-digest", Plan: "@every 30s", Prompt: "summarise", Enabled: true}
	now := time.Unix(1_700_000_015, 0)

	got, err := led.Due(sc, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("first due = %d triggers", len(got))
	}
	if emitted, _ := led.Emit(got[0]); !emitted {
		t.Fatal("first emit refused")
	}

	// Same instant inside the same window: nothing owed.
	if again, _ := led.Due(sc, now.Add(5*time.Second)); len(again) != 0 {
		t.Fatalf("same window re-fired: %+v", again)
	}

	// A restart must not replay the instant either.
	reopened, err := OpenLedger(path)
	if err != nil {
		t.Fatal(err)
	}
	if replay, _ := reopened.Due(sc, now); len(replay) != 0 {
		t.Fatalf("restart replayed %+v", replay)
	}

	// The next window does fire.
	next, _ := reopened.Due(sc, now.Add(30*time.Second))
	if len(next) != 1 {
		t.Fatalf("next window = %d", len(next))
	}
}

func TestDedupKeyIsIdempotent(t *testing.T) {
	led, err := OpenLedger(filepath.Join(t.TempDir(), "t.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	tr := Trigger{ID: "x", DedupKey: "same-key", Prompt: "p"}
	ok, err := led.Emit(tr)
	if err != nil || !ok {
		t.Fatalf("first emit ok=%v err=%v", ok, err)
	}
	ok, err = led.Emit(tr)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("duplicate dedup_key was accepted")
	}
	if n := len(led.List()); n != 1 {
		t.Fatalf("ledger has %d triggers, want 1", n)
	}
}

func TestDailyPlan(t *testing.T) {
	led, _ := OpenLedger(filepath.Join(t.TempDir(), "d.jsonl"))
	sc := Schedule{ID: "s", Plan: "daily 09:00", Enabled: true}
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.Local)
	got, err := led.Due(sc, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("due = %d", len(got))
	}
	if at := time.Unix(got[0].At, 0); at.Hour() != 9 || at.Minute() != 0 || at.Day() != 4 {
		t.Fatalf("fired at %v", at)
	}
}
