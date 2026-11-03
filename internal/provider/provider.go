// Package provider defines the model-agnostic streaming interface that the
// whole agent base is built on.
//
// Everything downstream (agent loop, CLI, desktop shell) talks to this
// interface only. Adding a vendor means writing one Stream implementation and
// registering it; it never means touching the loop or the UI.
package provider

import (
	"context"
	"encoding/json"
	"errors"
)

// Message roles used by the OpenAI-compatible wire format.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCall is a model-initiated request to run a local tool.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolSpec declares one tool to the model using JSON Schema.
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Message is one entry in the conversation history.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Reasoning  string     `json:"reasoning,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// Request is a single streaming completion request.
type Request struct {
	Model       string
	Messages    []Message
	Tools       []ToolSpec
	MaxTokens   int
	Temperature *float64
	// SessionID is forwarded to providers that require a per-conversation
	// identity header (OpenCode Go uses x-opencode-session).
	SessionID string
}

// EventType discriminates stream events.
type EventType string

const (
	// EventReasoning carries model thinking tokens. It is deliberately a
	// different type from EventContent so the UI can collapse it.
	EventReasoning EventType = "reasoning"
	EventContent   EventType = "content"
	EventToolCall  EventType = "tool_call"
	EventToolStart EventType = "tool_start"
	EventToolDone  EventType = "tool_done"
	EventUsage     EventType = "usage"
	EventDone      EventType = "done"
	EventError     EventType = "error"
)

// Usage is the token accounting reported by the provider.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Event is one item on the stream.
type Event struct {
	Type     EventType `json:"type"`
	Text     string    `json:"text,omitempty"`
	ToolCall *ToolCall `json:"toolCall,omitempty"`
	Usage    *Usage    `json:"usage,omitempty"`
	Err      error     `json:"-"`
}

// Provider is the only interface the rest of the codebase depends on.
type Provider interface {
	Name() string
	DefaultModel() string
	Models() []string
	// Stream returns a channel closed by the producer. Errors that happen
	// mid-stream arrive as EventError events before the close; errors that
	// prevent the request from starting are returned directly.
	Stream(ctx context.Context, req Request) (<-chan Event, error)
}

// ErrNoProvider is returned when a caller asks for a provider that is not
// registered.
var ErrNoProvider = errors.New("provider: not registered")
