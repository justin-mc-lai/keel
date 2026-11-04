package perm

import (
	"context"
	"testing"
)

func TestModeDefaultAndSessionAllow(t *testing.T) {
	asked := 0
	c := New(func(ctx context.Context, sessionID, name string, args []byte) (Decision, error) {
		asked++
		return AllowSession, nil
	})
	if c.Mode() != Ask {
		t.Fatalf("default mode = %q, want ask", c.Mode())
	}
	if err := c.SetMode(Allow); err != nil {
		t.Fatal(err)
	}

	approve := c.Approver("s1")
	if !approve(context.Background(), "danger", nil) {
		t.Fatal("first call should be allowed")
	}
	if asked != 1 {
		t.Fatalf("asked = %d, want 1", asked)
	}

	// Same session, same tool: remembered, no second prompt.
	if !approve(context.Background(), "danger", nil) {
		t.Fatal("second call should be allowed")
	}
	if asked != 1 {
		t.Fatalf("asked = %d after remember, want 1", asked)
	}

	// Different tool in the same session still asks.
	if !approve(context.Background(), "other", nil) {
		t.Fatal("other tool should be allowed")
	}
	if asked != 2 {
		t.Fatalf("asked = %d, want 2", asked)
	}

	// A new session forgets.
	if c.Preapproved("s2", "danger") {
		t.Fatal("s2 must not inherit s1 approvals")
	}
}

func TestUnknownModeRejected(t *testing.T) {
	if _, err := NormalizeMode("yolo"); err == nil {
		t.Fatal("expected error for unknown mode")
	}
	if m, err := NormalizeMode(" BYPASS "); err != nil || m != Bypass {
		t.Fatalf("normalize = %q, %v", m, err)
	}
}

func TestBypassNeverAsks(t *testing.T) {
	asked := 0
	c := New(func(ctx context.Context, sessionID, name string, args []byte) (Decision, error) {
		asked++
		return Deny, nil
	})
	_ = c.SetMode(Bypass)
	if !c.Approver("s")(context.Background(), "danger", nil) {
		t.Fatal("bypass should allow")
	}
	if asked != 0 {
		t.Fatalf("bypass asked %d times", asked)
	}
	if !c.Warning() {
		t.Fatal("bypass must report warning")
	}
}
