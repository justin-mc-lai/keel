// Package tool is the local capability registry the agent may call.
//
// A tool only exists if it was registered. Anything the model invents is
// refused, and tools flagged as needing approval stay refused until an
// approver says yes.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/justin-mc-lai/keel/internal/jobs"
	"github.com/justin-mc-lai/keel/internal/provider"
)

// ErrUnknownTool is returned for a call the registry does not know.
var ErrUnknownTool = errors.New("tool: not registered")

// ErrDenied is returned when a tool exists but was not approved.
var ErrDenied = errors.New("tool: approval denied")

// Func is a tool implementation.
type Func func(ctx context.Context, args json.RawMessage) (string, error)

// Tool is one registered capability.
type Tool struct {
	Name             string
	Description      string
	Parameters       json.RawMessage
	RequiresApproval bool
	Run              Func
}

// Approver decides whether a call may run. Returning false denies it.
type Approver func(ctx context.Context, name string, args []byte) bool

// Preview describes a pending file change, before it touches disk.
type Preview struct {
	Path   string
	Before string
	After  string
}

// Previewer is called before a write-side tool commits. A non-nil error
// aborts the write.
type Previewer func(Preview) error

// Registry holds the tools and the approval policy.
type Registry struct {
	mu       sync.RWMutex
	tools    map[string]Tool
	approve  Approver
	preview  Previewer
	jobs     *jobs.Manager
	jobWait  time.Duration
	subagent func(ctx context.Context, task string) (string, error)
}

// NewRegistry builds an empty registry. A nil approver means every tool that
// requires approval is denied.
func NewRegistry(approve Approver) *Registry {
	return &Registry{
		tools:   map[string]Tool{},
		approve: approve,
		jobs:    jobs.New(jobs.DefaultLimit),
		jobWait: 5 * time.Second,
	}
}

// Register adds a tool. A tool without a name or a function is rejected.
func (r *Registry) Register(t Tool) error {
	if t.Name == "" {
		return errors.New("tool: empty name")
	}
	if t.Run == nil {
		return fmt.Errorf("tool %s: nil function", t.Name)
	}
	if len(t.Parameters) == 0 {
		t.Parameters = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name] = t
	return nil
}

// Specs returns the tool declarations sent to the model, sorted by name so the
// prompt is deterministic.
func (r *Registry) Specs() []provider.ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]provider.ToolSpec, 0, len(names))
	for _, n := range names {
		t := r.tools[n]
		out = append(out, provider.ToolSpec{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}
	return out
}

// SetPreview installs the preview hook used by write-side tools.
func (r *Registry) SetPreview(p Previewer) {
	r.mu.Lock()
	r.preview = p
	r.mu.Unlock()
}

// previewChange runs the hook (if any) before a write.
func (r *Registry) previewChange(p Preview) error {
	r.mu.RLock()
	fn := r.preview
	r.mu.RUnlock()
	if fn == nil {
		return nil
	}
	return fn(p)
}

// SetSubagent installs the runner behind spawn_subagent. Without it the tool
// is not registered at all, so a model never sees a capability that is missing.
func (r *Registry) SetSubagent(fn func(ctx context.Context, task string) (string, error)) error {
	r.mu.Lock()
	r.subagent = fn
	r.mu.Unlock()
	return r.Register(Tool{
		Name:        "spawn_subagent",
		Description: "Run one self-contained task in an isolated session and return its result.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"task":{"type":"string"}},"required":["task"]}`),
		Run: func(ctx context.Context, args json.RawMessage) (string, error) {
			var in struct {
				Task string `json:"task"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return "", err
			}
			if in.Task == "" {
				return "", fmt.Errorf("spawn_subagent: empty task")
			}
			return fn(ctx, in.Task)
		},
	})
}

// Names lists registered tool names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Run executes a tool call. Unknown tools and denied approvals never reach the
// implementation.
func (r *Registry) Run(ctx context.Context, call provider.ToolCall) (string, error) {
	r.mu.RLock()
	t, ok := r.tools[call.Name]
	approve := r.approve
	r.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnknownTool, call.Name)
	}
	args := json.RawMessage(call.Arguments)
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	if !json.Valid(args) {
		return "", fmt.Errorf("tool %s: invalid JSON arguments", call.Name)
	}
	if t.RequiresApproval {
		if approve == nil || !approve(ctx, t.Name, []byte(args)) {
			return "", fmt.Errorf("%w: %s", ErrDenied, call.Name)
		}
	}
	return t.Run(ctx, args)
}
