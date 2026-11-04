package store

import (
	"encoding/json"
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

func TestSessionManage(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a, _ := st.CreateSession("alpha")
	b, _ := st.CreateSession("beta")
	_ = st.AppendMessage(a, provider.Message{Role: provider.RoleUser, Content: "hi"})

	if err := st.RenameSession(a, "renamed alpha"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	hits, err := st.SearchSessions("renamed")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != a {
		t.Fatalf("search = %+v", hits)
	}
	if hits, _ := st.SearchSessions("beta"); len(hits) != 1 || hits[0].ID != b {
		t.Fatalf("search beta = %+v", hits)
	}

	raw, err := st.ExportSession(a)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var got struct {
		Session  Session            `json:"session"`
		Messages []provider.Message `json:"messages"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("export is not JSON: %v", err)
	}
	if got.Session.ID != a || len(got.Messages) != 1 {
		t.Fatalf("export = %+v", got)
	}

	if err := st.DeleteSession(a); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, _ := st.ListSessions()
	if len(list) != 1 || list[0].ID != b {
		t.Fatalf("after delete = %+v", list)
	}
	if msgs, _ := st.Messages(a); len(msgs) != 0 {
		t.Fatalf("messages survived delete: %+v", msgs)
	}
}

func TestRetryTruncates(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sid, _ := st.CreateSession("t")

	_ = st.AppendMessage(sid, provider.Message{Role: provider.RoleUser, Content: "first"})
	_ = st.AppendMessage(sid, provider.Message{Role: provider.RoleAssistant, Content: "one"})
	_ = st.AppendMessage(sid, provider.Message{Role: provider.RoleUser, Content: "second"})
	_ = st.AppendMessage(sid, provider.Message{Role: provider.RoleAssistant, Content: "two"})
	_ = st.AppendMessage(sid, provider.Message{Role: provider.RoleTool, Content: "out", ToolCallID: "c1"})

	id, text, err := st.LastUserMessage(sid)
	if err != nil {
		t.Fatal(err)
	}
	if text != "second" {
		t.Fatalf("last user = %q", text)
	}
	if err := st.TruncateAfter(sid, id-1); err != nil {
		t.Fatal(err)
	}
	msgs, _ := st.Messages(sid)
	if len(msgs) != 2 {
		t.Fatalf("after truncate = %d messages", len(msgs))
	}
	if msgs[0].Content != "first" || msgs[1].Content != "one" {
		t.Fatalf("wrong survivors: %+v", msgs)
	}
}
