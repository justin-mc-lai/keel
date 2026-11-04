package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Builtins returns the default safe tool set. Both are side-effect free, which
// is why neither requires approval.
func Builtins() []Tool {
	return []Tool{
		{
			Name:        "get_time",
			Description: "Return the current time in an IANA timezone (e.g. Asia/Shanghai). Defaults to UTC.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"tz":{"type":"string","description":"IANA timezone name"}}}`),
			Run: func(ctx context.Context, args json.RawMessage) (string, error) {
				var in struct {
					TZ string `json:"tz"`
				}
				_ = json.Unmarshal(args, &in)
				if in.TZ == "" {
					in.TZ = "UTC"
				}
				loc, err := time.LoadLocation(in.TZ)
				if err != nil {
					return "", fmt.Errorf("unknown timezone %q", in.TZ)
				}
				return time.Now().In(loc).Format(time.RFC3339), nil
			},
		},
		{
			// The canonical gated tool: useful, and the reason the permission
			// tiers exist. 30s ceiling, output capped so a runaway command
			// cannot flood the conversation.
			Name:             "run_shell",
			Description:      "Run a shell command locally and return its combined output. Requires approval.",
			Parameters:       json.RawMessage(`{"type":"object","properties":{"command":{"type":"string","description":"shell command to run"}},"required":["command"]}`),
			RequiresApproval: true,
			Run: func(ctx context.Context, args json.RawMessage) (string, error) {
				var in struct {
					Command string `json:"command"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return "", err
				}
				if strings.TrimSpace(in.Command) == "" {
					return "", fmt.Errorf("empty command")
				}
				cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				cmd := exec.CommandContext(cctx, "sh", "-c", in.Command)
				// WaitDelay forcibly breaks the Wait when a grandchild keeps the
				// pipe open after the context is done.
				cmd.WaitDelay = 2 * time.Second
				setupProc(cmd)
				out := &cappedBuffer{max: 8192}
				cmd.Stdout = out
				cmd.Stderr = out
				err := cmd.Run()
				text := out.String()
				if out.truncated {
					text += "\n[output truncated]"
				}
				if err != nil {
					return text, fmt.Errorf("%v: %s", err, strings.TrimSpace(text))
				}
				return text, nil
			},
		},
		{
			Name:        "echo",
			Description: "Return the given text unchanged. Useful for verifying the tool round trip.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`),
			Run: func(ctx context.Context, args json.RawMessage) (string, error) {
				var in struct {
					Text string `json:"text"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return "", err
				}
				return in.Text, nil
			},
		},
	}
}

// NewDefaultRegistry registers the built-ins and applies the approval policy.
func NewDefaultRegistry(approve Approver) (*Registry, error) {
	r := NewRegistry(approve)
	for _, t := range Builtins() {
		if err := r.Register(t); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// cappedBuffer stops a runaway command from flooding memory or the transcript.
type cappedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.buf.Len() < c.max {
		room := c.max - c.buf.Len()
		if len(p) > room {
			c.buf.Write(p[:room])
			c.truncated = true
		} else {
			c.buf.Write(p)
		}
	} else if len(p) > 0 {
		c.truncated = true
	}
	// Report full consumption so the child is never blocked by a short write.
	return len(p), nil
}

func (c *cappedBuffer) String() string { return c.buf.String() }
