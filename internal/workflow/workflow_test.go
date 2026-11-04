package workflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func def(nodes ...Node) *Definition { return &Definition{ID: "wf", Nodes: nodes} }

func TestDefinitionRejectsBadGraphs(t *testing.T) {
	cases := map[string]*Definition{
		"duplicate": def(
			Node{ID: "a", Type: NodeDelay, Duration: "1ms"},
			Node{ID: "a", Type: NodeDelay, Duration: "1ms"},
		),
		"missing dep": def(
			Node{ID: "a", Type: NodeDelay, Depends: []string{"ghost"}},
		),
		"cycle": def(
			Node{ID: "a", Type: NodeDelay, Depends: []string{"b"}},
			Node{ID: "b", Type: NodeDelay, Depends: []string{"a"}},
		),
		"unknown type": def(Node{ID: "a", Type: "teleport"}),
		"empty":        def(),
	}
	for name, d := range cases {
		if err := d.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}

	ok := def(
		Node{ID: "a", Type: NodeDelay, Duration: "1ms"},
		Node{ID: "b", Type: NodeDelay, Duration: "1ms", Depends: []string{"a"}},
	)
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid graph rejected: %v", err)
	}
	order, err := ok.Order()
	if err != nil || len(order) != 2 || order[0] != "a" {
		t.Fatalf("order = %v err=%v", order, err)
	}
}

func TestEngineRetriesAndTimesOut(t *testing.T) {
	var calls int32
	d := def(Node{
		ID: "flaky", Type: NodeAgent,
		Retry: &Retry{Attempts: 3, Backoff: "1ms"},
	})
	run := Execute(context.Background(), d, Deps{
		Agent: func(ctx context.Context, prompt string) (string, error) {
			if atomic.AddInt32(&calls, 1) < 3 {
				return "", errors.New("boom")
			}
			return "ok", nil
		},
	})
	if run.Status != "passed" || run.Nodes[0].Attempts != 3 {
		t.Fatalf("run = %+v", run)
	}

	// A node that never succeeds fails the run, and later nodes never run.
	var later int32
	d2 := def(
		Node{ID: "bad", Type: NodeAgent, Retry: &Retry{Attempts: 2, Backoff: "1ms"}},
		Node{ID: "after", Type: NodeDelay, Duration: "1ms", Depends: []string{"bad"}},
	)
	run2 := Execute(context.Background(), d2, Deps{
		Agent: func(ctx context.Context, prompt string) (string, error) { return "", errors.New("nope") },
	})
	if run2.Status != "failed" {
		t.Fatalf("status = %s", run2.Status)
	}
	if run2.Nodes[0].Attempts != 2 {
		t.Fatalf("attempts = %d", run2.Nodes[0].Attempts)
	}
	if atomic.LoadInt32(&later) != 0 || len(run2.Nodes) != 1 {
		t.Fatalf("dependent node ran after a failure: %+v", run2.Nodes)
	}

	// Timeout is a failure, not a hang.
	d3 := def(Node{ID: "slow", Type: NodeAgent, Timeout: "30ms", Retry: &Retry{Attempts: 1}})
	start := time.Now()
	run3 := Execute(context.Background(), d3, Deps{
		Agent: func(ctx context.Context, prompt string) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		},
	})
	if run3.Status != "failed" {
		t.Fatalf("timeout status = %s", run3.Status)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout took %v", elapsed)
	}
}

func TestEngineNodesAndTemplates(t *testing.T) {
	d := def(
		Node{ID: "fetch", Type: NodeHTTP, URL: "https://example.test/data"},
		Node{ID: "think", Type: NodeAgent, Prompt: "summarise: {{ fetch }}", Depends: []string{"fetch"}},
		Node{ID: "wait", Type: NodeDelay, Duration: "1ms", Depends: []string{"think"}},
	)

	var gotPrompt string
	run := Execute(context.Background(), d, Deps{
		HTTP: func(ctx context.Context, url string) (string, error) {
			if url != "https://example.test/data" {
				t.Fatalf("url = %q", url)
			}
			return "RAW DATA", nil
		},
		Agent: func(ctx context.Context, prompt string) (string, error) {
			gotPrompt = prompt
			return "SUMMARY", nil
		},
	})

	if run.Status != "passed" {
		t.Fatalf("run = %+v", run)
	}
	if gotPrompt != "summarise: RAW DATA" {
		t.Fatalf("template not substituted: %q", gotPrompt)
	}
	if len(run.Nodes) != 3 {
		t.Fatalf("nodes = %d", len(run.Nodes))
	}
	if run.Nodes[1].Output != "SUMMARY" {
		t.Fatalf("output = %q", run.Nodes[1].Output)
	}
}

func TestRunRecordPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.json")
	t.Setenv("AGENTDESK_RUNS", path)

	d := def(Node{ID: "a", Type: NodeDelay, Duration: "1ms"})
	run := Execute(context.Background(), d, Deps{})

	st, err := OpenStore("")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save(*run); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"status": "passed"`) {
		t.Fatalf("record not written: %s", raw)
	}

	list, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Workflow != "wf" || len(list[0].Nodes) != 1 {
		t.Fatalf("round trip = %+v", list)
	}
}
