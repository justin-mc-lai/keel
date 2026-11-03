package store

import (
	"path/filepath"
	"testing"

	"github.com/justin-mc-lai/agentdesk/internal/provider"
)

func TestSessionPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sid, err := st.CreateSession("hello")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := st.AppendMessage(sid, provider.Message{Role: provider.RoleUser, Content: "hi"}); err != nil {
		t.Fatalf("append user: %v", err)
	}
	if err := st.AppendMessage(sid, provider.Message{
		Role:      provider.RoleAssistant,
		Content:   "calling",
		Reasoning: "because",
		ToolCalls: []provider.ToolCall{{ID: "c1", Name: "echo", Arguments: `{"text":"hi"}`}},
	}); err != nil {
		t.Fatalf("append assistant: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	re, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer re.Close()

	msgs, err := re.Messages(sid)
	if err != nil {
		t.Fatalf("messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "hi" {
		t.Fatalf("first = %+v", msgs[0])
	}
	if msgs[1].Reasoning != "because" {
		t.Fatalf("reasoning not persisted: %+v", msgs[1])
	}
	if len(msgs[1].ToolCalls) != 1 || msgs[1].ToolCalls[0].Name != "echo" {
		t.Fatalf("tool calls not persisted: %+v", msgs[1])
	}

	list, err := re.ListSessions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].ID != sid {
		t.Fatalf("sessions = %+v", list)
	}
}
