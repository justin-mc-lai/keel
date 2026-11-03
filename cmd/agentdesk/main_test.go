package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/justin-mc-lai/agentdesk/internal/provider"
	"github.com/justin-mc-lai/agentdesk/internal/store"
)

type fakeStreamer struct{ reply string }

func (f *fakeStreamer) Name() string         { return "fake" }
func (f *fakeStreamer) DefaultModel() string { return "fake-1" }
func (f *fakeStreamer) Models() []string     { return []string{"fake-1"} }

func (f *fakeStreamer) Stream(ctx context.Context, req provider.Request) (<-chan provider.Event, error) {
	ch := make(chan provider.Event, 4)
	go func() {
		defer close(ch)
		for _, part := range []string{f.reply[:2], f.reply[2:]} {
			ch <- provider.Event{Type: provider.EventContent, Text: part}
		}
		ch <- provider.Event{Type: provider.EventDone}
	}()
	return ch, nil
}

func TestRunCommandStreams(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sid, err := st.CreateSession("t")
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	answer, err := runPrompt(context.Background(), &fakeStreamer{reply: "PONG"}, st, sid, "hi", "", false, &out)
	if err != nil {
		t.Fatalf("runPrompt: %v", err)
	}
	if answer != "PONG" {
		t.Fatalf("answer = %q", answer)
	}
	if got := out.String(); got != "PONG" {
		t.Fatalf("streamed output = %q", got)
	}
}

func TestExecuteUnknownCommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Execute([]string{"nope"}, &out, &errOut); code != 2 {
		t.Fatalf("exit code = %d", code)
	}
	if !bytes.Contains(errOut.Bytes(), []byte("unknown command")) {
		t.Fatalf("stderr = %q", errOut.String())
	}
}
