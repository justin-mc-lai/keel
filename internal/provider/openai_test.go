package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sseBody = `data: {"choices":[{"delta":{"reasoning_content":"thinking"}}]}

data: {"choices":[{"delta":{"content":"PONG"}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"echo","arguments":"{\"text\":"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"hi\"}"}}]}}]}

data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":11,"completion_tokens":3,"total_tokens":14}}

data: [DONE]

`

func TestStreamSplitsReasoningAndContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-opencode-session"); got != "sess-1" {
			t.Errorf("session header = %q, want sess-1", got)
		}
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(sseBody))
	}))
	defer srv.Close()

	p := NewOpenAICompatible(OpenAICompatibleOptions{
		Name:          "fake",
		BaseURL:       srv.URL,
		APIKey:        "test",
		SessionHeader: "x-opencode-session",
		DefaultModel:  "m1",
	})
	ch, err := p.Stream(context.Background(), Request{
		Messages:  []Message{{Role: RoleUser, Content: "hi"}},
		SessionID: "sess-1",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var reasoning, content []string
	var calls []ToolCall
	var usage *Usage
	done := false
	for ev := range ch {
		switch ev.Type {
		case EventReasoning:
			reasoning = append(reasoning, ev.Text)
		case EventContent:
			content = append(content, ev.Text)
		case EventToolCall:
			calls = append(calls, *ev.ToolCall)
		case EventUsage:
			usage = ev.Usage
		case EventDone:
			done = true
		case EventError:
			t.Fatalf("stream error: %v", ev.Err)
		}
	}

	if !done {
		t.Fatal("no done event")
	}
	if len(reasoning) == 0 || reasoning[0] != "thinking" {
		t.Fatalf("reasoning events = %v", reasoning)
	}
	if len(content) == 0 || strings.Join(content, "") != "PONG" {
		t.Fatalf("content events = %v", content)
	}
	if len(calls) != 1 {
		t.Fatalf("tool calls = %v", calls)
	}
	if calls[0].Name != "echo" || calls[0].Arguments != `{"text":"hi"}` {
		t.Fatalf("tool call = %+v", calls[0])
	}
	if usage == nil || usage.TotalTokens != 14 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestStreamSurfacesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()

	p := NewOpenAICompatible(OpenAICompatibleOptions{Name: "fake", BaseURL: srv.URL})
	_, err := p.Stream(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("error = %v", err)
	}
}

func TestStreamRetriesOnConnectionDrop(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			// Drop the connection before any response header: the case the
			// retry loop exists for.
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("expected hijacker")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			conn.Close()
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"PONG\"}}]}\n\ndata: [DONE]\n\n"))
	}))
	defer srv.Close()

	p := NewOpenAICompatible(OpenAICompatibleOptions{Name: "flaky", BaseURL: srv.URL})
	ch, err := p.Stream(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("Stream should have retried: %v", err)
	}
	var got strings.Builder
	for ev := range ch {
		if ev.Type == EventContent {
			got.WriteString(ev.Text)
		}
	}
	if got.String() != "PONG" {
		t.Fatalf("content = %q", got.String())
	}
	if hits < 2 {
		t.Fatalf("expected a retry, hits = %d", hits)
	}
}
