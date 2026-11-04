package agent

import (
	"context"
	"fmt"
)

// SubagentPrompt keeps a child focused: it returns a summary, not a transcript.
const SubagentPrompt = `You are a focused sub-agent. Complete the assigned task and reply with a short result.
Do not ask follow-up questions. Do not describe your process.`

// SubagentFunc runs one task in an isolated session.
type SubagentFunc func(ctx context.Context, task string) (string, error)

// Subagent returns a runner that executes tasks in a fresh session of the same
// store, provider and model.
//
// The child gets its own message history: the parent never sees the child's
// intermediate turns, and the child never sees the parent's. That isolation is
// the whole point — a sub-agent is a clean context, not a fork of the chat.
func (l *Loop) Subagent() SubagentFunc {
	return func(ctx context.Context, task string) (string, error) {
		if l.opt.Store == nil {
			return "", fmt.Errorf("subagent: no store configured")
		}
		sid, err := l.opt.Store.CreateSession("sub: " + titleFor(task))
		if err != nil {
			return "", err
		}
		child := New(Options{
			Provider:      l.opt.Provider,
			Tools:         l.opt.Tools,
			Store:         l.opt.Store,
			Model:         l.opt.Model,
			Effort:        l.opt.Effort,
			SystemPrompt:  SubagentPrompt,
			MaxIterations: l.opt.MaxIterations,
		})
		return child.Run(ctx, sid, task)
	}
}
