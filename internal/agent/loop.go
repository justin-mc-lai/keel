// Package agent is the whole agent base: one loop, tool rounds, and an event
// sink. It knows nothing about the UI and nothing about any specific vendor.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/justin-mc-lai/agentdesk/internal/provider"
	"github.com/justin-mc-lai/agentdesk/internal/store"
	"github.com/justin-mc-lai/agentdesk/internal/tool"
)

// DefaultSystemPrompt keeps the assistant grounded in what it can actually do.
const DefaultSystemPrompt = `You are a concise, tool-using assistant running inside a local desktop app.
Use the provided tools when they are the honest way to answer. If a tool fails, say so plainly instead of inventing a result.`

// Options configures a Loop.
type Options struct {
	Provider      provider.Provider
	Tools         *tool.Registry
	Store         *store.Store
	Model         string
	Effort        string
	Instructions  string
	SystemPrompt  string
	MaxIterations int
	OnEvent       func(provider.Event)
}

// Loop runs one conversation turn at a time.
type Loop struct {
	opt Options
}

// New builds a loop, filling in defaults.
func New(o Options) *Loop {
	if o.MaxIterations <= 0 {
		o.MaxIterations = 8
	}
	if o.SystemPrompt == "" {
		o.SystemPrompt = DefaultSystemPrompt
	}
	return &Loop{opt: o}
}

// Run streams one user turn to completion, executing tool rounds as needed.
// It returns the final assistant text.
func (l *Loop) Run(ctx context.Context, sessionID, input string) (string, error) {
	if l.opt.Provider == nil {
		return "", fmt.Errorf("agent: no provider configured")
	}
	if l.opt.Store == nil {
		return "", fmt.Errorf("agent: no store configured")
	}
	if err := l.opt.Store.EnsureSession(sessionID, titleFor(input)); err != nil {
		return "", err
	}
	user := provider.Message{Role: provider.RoleUser, Content: input}
	if err := l.opt.Store.AppendMessage(sessionID, user); err != nil {
		return "", err
	}

	history, err := l.opt.Store.Messages(sessionID)
	if err != nil {
		return "", err
	}
	msgs := make([]provider.Message, 0, len(history)+1)
	if sys := l.systemPrompt(); sys != "" {
		msgs = append(msgs, provider.Message{Role: provider.RoleSystem, Content: sys})
	}
	msgs = append(msgs, history...)

	for iter := 0; iter < l.opt.MaxIterations; iter++ {
		stream, err := l.opt.Provider.Stream(ctx, provider.Request{
			Model:     l.opt.Model,
			Effort:    l.opt.Effort,
			Messages:  msgs,
			Tools:     l.tools(),
			SessionID: sessionID,
		})
		if err != nil {
			return "", err
		}

		var content, reasoning strings.Builder
		var calls []provider.ToolCall
		for ev := range stream {
			switch ev.Type {
			case provider.EventContent:
				content.WriteString(ev.Text)
			case provider.EventReasoning:
				reasoning.WriteString(ev.Text)
			case provider.EventToolCall:
				if ev.ToolCall != nil {
					calls = append(calls, *ev.ToolCall)
				}
			case provider.EventError:
				if ev.Err != nil {
					return "", ev.Err
				}
			}
			l.emit(ev)
		}

		assistant := provider.Message{
			Role:      provider.RoleAssistant,
			Content:   content.String(),
			Reasoning: reasoning.String(),
			ToolCalls: calls,
		}
		if err := l.opt.Store.AppendMessage(sessionID, assistant); err != nil {
			return "", err
		}
		msgs = append(msgs, assistant)

		if len(calls) == 0 {
			return assistant.Content, nil
		}

		for _, call := range calls {
			l.emit(provider.Event{Type: provider.EventToolStart, ToolCall: &call})
			out, runErr := l.runTool(ctx, call)
			text := out
			if runErr != nil {
				text = denialText(runErr)
			}
			l.emit(provider.Event{Type: provider.EventToolDone, ToolCall: &call, Text: text})
			toolMsg := provider.Message{
				Role:       provider.RoleTool,
				Content:    text,
				ToolCallID: call.ID,
				Name:       call.Name,
			}
			if err := l.opt.Store.AppendMessage(sessionID, toolMsg); err != nil {
				return "", err
			}
			msgs = append(msgs, toolMsg)
		}
	}

	return "", fmt.Errorf("agent: exceeded %d iterations without a final answer", l.opt.MaxIterations)
}

// systemPrompt merges the built-in prompt with the workspace AGENTS.md text.
func (l *Loop) systemPrompt() string {
	sys := strings.TrimSpace(l.opt.SystemPrompt)
	if ins := strings.TrimSpace(l.opt.Instructions); ins != "" {
		if sys != "" {
			sys += "\n\n"
		}
		sys += "# Workspace instructions (AGENTS.md)\n" + ins
	}
	return sys
}

func (l *Loop) tools() []provider.ToolSpec {
	if l.opt.Tools == nil {
		return nil
	}
	return l.opt.Tools.Specs()
}

// denialText keeps the transcript string stable for the two cases the UI and
// the acceptance criteria name explicitly.
func denialText(err error) string {
	if errors.Is(err, tool.ErrDenied) {
		return "error: tool: approval denied"
	}
	if errors.Is(err, tool.ErrUnknownTool) {
		return "error: tool: not registered"
	}
	return "error: " + err.Error()
}

func (l *Loop) runTool(ctx context.Context, call provider.ToolCall) (string, error) {
	if l.opt.Tools == nil {
		return "", tool.ErrUnknownTool
	}
	return l.opt.Tools.Run(ctx, call)
}

func (l *Loop) emit(ev provider.Event) {
	if l.opt.OnEvent != nil {
		l.opt.OnEvent(ev)
	}
}

func titleFor(input string) string {
	s := strings.TrimSpace(strings.ReplaceAll(input, "\n", " "))
	if s == "" {
		return "New chat"
	}
	r := []rune(s)
	if len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return s
}
