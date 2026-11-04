package jobs

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestLongCommandDetachesAndReturns(t *testing.T) {
	m := New(30 * time.Second)

	id, err := m.Start("sleep 0.4; echo finished", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "job_") {
		t.Fatalf("id = %q", id)
	}

	// Immediately after start the job is still running.
	out, done, err := m.Poll(id)
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Fatal("job finished too fast to be detached")
	}
	if out != "" {
		t.Fatalf("unexpected early output %q", out)
	}

	// Waiting with a short window returns without blocking on completion.
	if _, done, _ := m.Wait(context.Background(), id, 50*time.Millisecond); done {
		t.Fatal("short wait should not see completion")
	}

	// Waiting long enough collects the output.
	out, done, err = m.Wait(context.Background(), id, 3*time.Second)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if !done || !strings.Contains(out, "finished") {
		t.Fatalf("done=%v out=%q", done, out)
	}
}

func TestKillStopsJob(t *testing.T) {
	m := New(30 * time.Second)
	id, err := m.Start("sleep 5", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Kill(id); err != nil {
		t.Fatal(err)
	}
	_, done, _ := m.Wait(context.Background(), id, 3*time.Second)
	if !done {
		t.Fatal("killed job should report done")
	}
}

func TestUnknownJob(t *testing.T) {
	m := New(time.Second)
	if _, _, err := m.Poll("job_nope"); err == nil {
		t.Fatal("expected error")
	}
}
