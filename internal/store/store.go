// Package store persists conversations in SQLite using a pure-Go driver, so
// the desktop binary needs no CGO and no external database.
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/justin-mc-lai/agentdesk/internal/provider"
)

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
  id         TEXT PRIMARY KEY,
  title      TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS messages (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id   TEXT NOT NULL,
  role         TEXT NOT NULL,
  content      TEXT NOT NULL DEFAULT '',
  reasoning    TEXT NOT NULL DEFAULT '',
  tool_calls   TEXT NOT NULL DEFAULT '',
  tool_call_id TEXT NOT NULL DEFAULT '',
  name         TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, id);
`

// Session is a conversation header.
type Session struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// Store owns the database handle.
type Store struct {
	db *sql.DB
}

// DefaultPath returns ~/.agentdesk/agentdesk.db.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "agentdesk.db"
	}
	return filepath.Join(home, ".agentdesk", "agentdesk.db")
}

// Open opens (and creates if needed) the database at path.
func Open(path string) (*Store, error) {
	if path == "" {
		path = DefaultPath()
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("store: create dir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the handle.
func (s *Store) Close() error { return s.db.Close() }

// CreateSession inserts a conversation and returns its id.
func (s *Store) CreateSession(title string) (string, error) {
	id, err := newID()
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()
	_, err = s.db.Exec(
		`INSERT INTO sessions(id, title, created_at, updated_at) VALUES(?,?,?,?)`,
		id, title, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("store: create session: %w", err)
	}
	return id, nil
}

// EnsureSession creates a session with the given id when it does not exist.
func (s *Store) EnsureSession(id, title string) error {
	if id == "" {
		return fmt.Errorf("store: empty session id")
	}
	now := time.Now().Unix()
	_, err := s.db.Exec(
		`INSERT INTO sessions(id, title, created_at, updated_at) VALUES(?,?,?,?)
		 ON CONFLICT(id) DO NOTHING`,
		id, title, now, now,
	)
	if err != nil {
		return fmt.Errorf("store: ensure session: %w", err)
	}
	return nil
}

// ListSessions returns conversations, newest first.
func (s *Store) ListSessions() ([]Session, error) {
	rows, err := s.db.Query(`SELECT id, title, created_at, updated_at FROM sessions ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: list sessions: %w", err)
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var it Session
		if err := rows.Scan(&it.ID, &it.Title, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// AppendMessage writes one message and bumps the session timestamp.
func (s *Store) AppendMessage(sessionID string, m provider.Message) error {
	toolCalls := ""
	if len(m.ToolCalls) > 0 {
		raw, err := json.Marshal(m.ToolCalls)
		if err != nil {
			return fmt.Errorf("store: encode tool calls: %w", err)
		}
		toolCalls = string(raw)
	}
	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO messages(session_id, role, content, reasoning, tool_calls, tool_call_id, name, created_at)
		 VALUES(?,?,?,?,?,?,?,?)`,
		sessionID, m.Role, m.Content, m.Reasoning, toolCalls, m.ToolCallID, m.Name, now,
	); err != nil {
		return fmt.Errorf("store: append message: %w", err)
	}
	if _, err := tx.Exec(`UPDATE sessions SET updated_at=? WHERE id=?`, now, sessionID); err != nil {
		return fmt.Errorf("store: touch session: %w", err)
	}
	return tx.Commit()
}

// Messages returns a session's history in insertion order.
func (s *Store) Messages(sessionID string) ([]provider.Message, error) {
	rows, err := s.db.Query(
		`SELECT role, content, reasoning, tool_calls, tool_call_id, name
		 FROM messages WHERE session_id=? ORDER BY id ASC`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("store: messages: %w", err)
	}
	defer rows.Close()
	var out []provider.Message
	for rows.Next() {
		var m provider.Message
		var toolCalls string
		if err := rows.Scan(&m.Role, &m.Content, &m.Reasoning, &toolCalls, &m.ToolCallID, &m.Name); err != nil {
			return nil, err
		}
		if toolCalls != "" {
			if err := json.Unmarshal([]byte(toolCalls), &m.ToolCalls); err != nil {
				return nil, fmt.Errorf("store: decode tool calls: %w", err)
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "s_" + hex.EncodeToString(b), nil
}

// RenameSession sets a conversation's title.
func (s *Store) RenameSession(id, title string) error {
	_, err := s.db.Exec(`UPDATE sessions SET title=?, updated_at=? WHERE id=?`, title, time.Now().Unix(), id)
	if err != nil {
		return fmt.Errorf("store: rename: %w", err)
	}
	return nil
}

// DeleteSession removes a conversation and its messages.
func (s *Store) DeleteSession(id string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM messages WHERE session_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE id=?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// SearchSessions matches session titles, newest first.
func (s *Store) SearchSessions(q string) ([]Session, error) {
	rows, err := s.db.Query(
		`SELECT id, title, created_at, updated_at FROM sessions
		 WHERE title LIKE ? ORDER BY updated_at DESC`, "%"+q+"%")
	if err != nil {
		return nil, fmt.Errorf("store: search: %w", err)
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var it Session
		if err := rows.Scan(&it.ID, &it.Title, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Export is the portable form of one conversation.
type Export struct {
	Session  Session            `json:"session"`
	Messages []provider.Message `json:"messages"`
}

// ExportSession serialises one conversation as JSON.
func (s *Store) ExportSession(id string) ([]byte, error) {
	list, err := s.ListSessions()
	if err != nil {
		return nil, err
	}
	var found *Session
	for i := range list {
		if list[i].ID == id {
			found = &list[i]
			break
		}
	}
	if found == nil {
		return nil, fmt.Errorf("store: session %s not found", id)
	}
	msgs, err := s.Messages(id)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(Export{Session: *found, Messages: msgs}, "", "  ")
}

// LastUserMessage returns the id and text of the newest user message.
func (s *Store) LastUserMessage(sessionID string) (int64, string, error) {
	var id int64
	var content string
	err := s.db.QueryRow(
		`SELECT id, content FROM messages WHERE session_id=? AND role=? ORDER BY id DESC LIMIT 1`,
		sessionID, provider.RoleUser).Scan(&id, &content)
	if err != nil {
		return 0, "", fmt.Errorf("store: last user message: %w", err)
	}
	return id, content, nil
}

// TruncateAfter deletes every message newer than id. Used by retry.
func (s *Store) TruncateAfter(sessionID string, id int64) error {
	_, err := s.db.Exec(`DELETE FROM messages WHERE session_id=? AND id>?`, sessionID, id)
	if err != nil {
		return fmt.Errorf("store: truncate: %w", err)
	}
	return nil
}
