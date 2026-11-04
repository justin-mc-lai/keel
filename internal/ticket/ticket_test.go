package ticket

import (
	"errors"
	"testing"
)

func TestFSMRejectsIllegalTransitions(t *testing.T) {
	want := []State{Open, Working, AwaitingPerson, AwaitingReview, Ended}
	for _, s := range want {
		if s == "" {
			t.Fatal("empty state")
		}
	}

	tk := New("t1", "title", "manual", "k1")
	if tk.State != Open {
		t.Fatalf("new state = %s", tk.State)
	}

	steps := []Event{Start, Submit, Accept}
	for _, ev := range steps {
		if err := tk.Apply(ev, ""); err != nil {
			t.Fatalf("%s: %v", ev, err)
		}
	}
	if tk.State != Ended {
		t.Fatalf("final state = %s", tk.State)
	}

	// Terminal states refuse everything.
	if err := tk.Apply(Start, ""); !errors.Is(err, ErrIllegal) {
		t.Fatalf("ended -> working should be illegal, got %v", err)
	}
	if tk.State != Ended {
		t.Fatal("illegal move mutated state")
	}

	// A move that skips the middle is refused too.
	other := New("t2", "x", "manual", "k2")
	if err := other.Apply(Accept, ""); !errors.Is(err, ErrIllegal) {
		t.Fatalf("open -> ended should be illegal, got %v", err)
	}
	if err := other.Apply(Submit, ""); !errors.Is(err, ErrIllegal) {
		t.Fatalf("open -> awaiting_review should be illegal, got %v", err)
	}

	// History records every accepted move.
	if len(tk.History) != 4 { // created + 3 moves
		t.Fatalf("history = %d", len(tk.History))
	}
}

func TestSlotsRespectLimitAndDrain(t *testing.T) {
	s := NewSlots(2)
	if !s.Acquire("a") || !s.Acquire("b") {
		t.Fatal("first two should start")
	}
	if s.Acquire("c") {
		t.Fatal("third must wait")
	}
	if r, q := s.Stats(); r != 2 || q != 1 {
		t.Fatalf("stats = %d/%d", r, q)
	}
	next, ok := s.Release("a")
	if !ok || next != "c" {
		t.Fatalf("release = %q %v", next, ok)
	}
	if r, q := s.Stats(); r != 2 || q != 0 {
		t.Fatalf("after drain = %d/%d", r, q)
	}
	if _, ok := s.Release("b"); ok {
		t.Fatal("nothing left to start")
	}
}
