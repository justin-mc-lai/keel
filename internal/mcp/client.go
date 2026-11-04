// Package mcp is a minimal Model Context Protocol client over stdio.
//
// Scope is deliberately narrow: spawn a server, initialize, list its tools,
// call one. No resources, no prompts, no sampling. A server that fails to start
// is reported and skipped — one broken integration must not take the agent down.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/justin-mc-lai/keel/internal/tool"
)

// ProtocolVersion is the MCP revision this client speaks.
const ProtocolVersion = "2024-11-05"

// ServerConfig describes one stdio server.
type ServerConfig struct {
	Name    string   `json:"name,omitempty"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Env     []string `json:"env,omitempty"`
	Dir     string   `json:"dir,omitempty"`
}

// ToolInfo is one tool announced by a server.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Client is one live server connection.
type Client struct {
	name   string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	reader *bufio.Reader

	mu     sync.Mutex
	nextID int
	tools  []ToolInfo
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

// Start launches a server, performs the initialize handshake and lists tools.
func Start(cfg ServerConfig) (*Client, error) {
	if strings.TrimSpace(cfg.Command) == "" {
		return nil, fmt.Errorf("mcp %s: empty command", cfg.Name)
	}
	cmd := exec.Command(cfg.Command, cfg.Args...)
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}
	if len(cfg.Env) > 0 {
		cmd.Env = append(cmd.Environ(), cfg.Env...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp %s: start: %w", cfg.Name, err)
	}

	c := &Client{name: cfg.Name, cmd: cmd, stdin: stdin, reader: bufio.NewReaderSize(stdout, 1<<20)}
	initParams := map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "keel", "version": tool.Version},
	}
	if _, err := c.call(context.Background(), "initialize", initParams); err != nil {
		c.Close()
		return nil, fmt.Errorf("mcp %s: initialize: %w", cfg.Name, err)
	}
	if err := c.notify("notifications/initialized", map[string]any{}); err != nil {
		c.Close()
		return nil, err
	}

	raw, err := c.call(context.Background(), "tools/list", map[string]any{})
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("mcp %s: tools/list: %w", cfg.Name, err)
	}
	var listed struct {
		Tools []ToolInfo `json:"tools"`
	}
	if err := json.Unmarshal(raw, &listed); err != nil {
		c.Close()
		return nil, fmt.Errorf("mcp %s: decode tools: %w", cfg.Name, err)
	}
	c.tools = listed.Tools
	return c, nil
}

// Name returns the configured server name.
func (c *Client) Name() string { return c.name }

// Tools returns the tools announced at startup.
func (c *Client) Tools() []ToolInfo { return append([]ToolInfo(nil), c.tools...) }

// Call invokes one tool and returns its text content.
func (c *Client) Call(ctx context.Context, name string, args json.RawMessage) (string, error) {
	if len(args) == 0 {
		args = json.RawMessage(`{}`)
	}
	params := map[string]any{"name": name, "arguments": json.RawMessage(args)}
	raw, err := c.call(ctx, "tools/call", params)
	if err != nil {
		return "", err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return string(raw), nil
	}
	var parts []string
	for _, part := range out.Content {
		if part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	text := strings.Join(parts, "\n")
	if out.IsError {
		return text, fmt.Errorf("%s", text)
	}
	return text, nil
}

// Close terminates the server process.
func (c *Client) Close() error {
	_ = c.stdin.Close()
	if c.cmd != nil && c.cmd.Process != nil {
		done := make(chan struct{})
		go func() { _ = c.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = c.cmd.Process.Kill()
		}
	}
	return nil
}

func (c *Client) notify(method string, params any) error {
	return c.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()

	if err := c.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}

	type result struct {
		raw json.RawMessage
		err error
	}
	ch := make(chan result, 1)
	go func() {
		for {
			line, err := c.reader.ReadBytes('\n')
			if err != nil {
				ch <- result{nil, fmt.Errorf("mcp %s: read: %w", c.name, err)}
				return
			}
			line = []byte(strings.TrimSpace(string(line)))
			if len(line) == 0 {
				continue
			}
			var resp rpcResponse
			if err := json.Unmarshal(line, &resp); err != nil {
				continue
			}
			if resp.ID != id {
				continue // notification or a stale reply
			}
			if resp.Error != nil {
				ch <- result{nil, fmt.Errorf("mcp %s: %s", c.name, resp.Error.Message)}
				return
			}
			ch <- result{resp.Result, nil}
			return
		}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		return res.raw, res.err
	}
}

func (c *Client) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err = c.stdin.Write(append(b, '\n'))
	return err
}

// StartAll starts every configured server. A failure is returned per server
// and never stops the others.
func StartAll(servers map[string]ServerConfig) ([]*Client, []error) {
	var clients []*Client
	var errs []error
	for name, cfg := range servers {
		if cfg.Name == "" {
			cfg.Name = name
		}
		c, err := Start(cfg)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		clients = append(clients, c)
	}
	return clients, errs
}

// RegisterTools adds every client's tools to one registry, namespaced by
// server so two servers can expose the same tool name.
func RegisterTools(reg *tool.Registry, clients []*Client) []error {
	var errs []error
	for _, client := range clients {
		client := client
		for _, info := range client.Tools() {
			info := info
			err := reg.Register(tool.Tool{
				Name:        "mcp_" + sanitize(client.Name()) + "_" + info.Name,
				Description: info.Description,
				Parameters:  info.InputSchema,
				Run: func(ctx context.Context, args json.RawMessage) (string, error) {
					return client.Call(ctx, info.Name, args)
				},
			})
			if err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errs
}

// Load is the one-shot form: start servers and register their tools.
func Load(reg *tool.Registry, servers map[string]ServerConfig) []error {
	clients, errs := StartAll(servers)
	return append(errs, RegisterTools(reg, clients)...)
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, s)
}
