package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/justin-mc-lai/agentdesk/internal/provider"
	"github.com/justin-mc-lai/agentdesk/internal/tool"
)

// TestMain doubles as an MCP server when the helper env var is set, so the
// test exercises a real subprocess and a real stdio handshake.
func TestMain(m *testing.M) {
	if os.Getenv("AGENTDESK_MCP_HELPER") == "1" {
		runHelperServer()
		return
	}
	os.Exit(m.Run())
}

func runHelperServer() {
	rd := bufio.NewReader(os.Stdin)
	for {
		line, err := rd.ReadBytes('\n')
		if err != nil {
			return
		}
		var req struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(line, &req) != nil {
			continue
		}
		if req.ID == 0 {
			continue // notification
		}
		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": ProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "helper", "version": "1"},
			}
		case "tools/list":
			result = map[string]any{"tools": []map[string]any{{
				"name":        "ping",
				"description": "Return pong with the given text.",
				"inputSchema": map[string]any{
					"type":       "object",
					"properties": map[string]any{"text": map[string]any{"type": "string"}},
					"required":   []string{"text"},
				},
			}}}
		case "tools/call":
			var p struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			if p.Name != "ping" {
				fmt.Fprintf(os.Stdout, `{"jsonrpc":"2.0","id":%d,"error":{"code":-32601,"message":"unknown tool"}}`+"\n", req.ID)
				continue
			}
			result = map[string]any{"content": []map[string]any{{
				"type": "text",
				"text": "pong:" + fmt.Sprint(p.Arguments["text"]),
			}}}
		default:
			fmt.Fprintf(os.Stdout, `{"jsonrpc":"2.0","id":%d,"error":{"code":-32601,"message":"unknown method"}}`+"\n", req.ID)
			continue
		}
		out, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
		fmt.Fprintf(os.Stdout, "%s\n", out)
	}
}

func helperConfig() ServerConfig {
	exe, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return ServerConfig{
		Name:    "helper",
		Command: exe,
		Args:    []string{"-test.run=^$"},
		Env:     []string{"AGENTDESK_MCP_HELPER=1"},
	}
}

func TestStdioServerRegistersTools(t *testing.T) {
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("no shell available")
	}
	client, err := Start(helperConfig())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer client.Close()

	tools := client.Tools()
	if len(tools) != 1 || tools[0].Name != "ping" {
		t.Fatalf("tools = %+v", tools)
	}
	if len(tools[0].InputSchema) == 0 {
		t.Fatal("missing schema")
	}

	out, err := client.Call(context.Background(), "ping", json.RawMessage(`{"text":"hi"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if out != "pong:hi" {
		t.Fatalf("output = %q", out)
	}

	// Registration into the shared tool registry uses a prefixed name.
	reg := tool.NewRegistry(nil)
	if errs := Load(reg, map[string]ServerConfig{"helper": helperConfig()}); len(errs) != 0 {
		t.Fatalf("load: %v", errs)
	}
	names := reg.Names()
	found := false
	for _, n := range names {
		if n == "mcp_helper_ping" {
			found = true
		}
	}
	if !found {
		t.Fatalf("registered names = %v", names)
	}
	res, err := reg.Run(context.Background(), provider.ToolCall{Name: "mcp_helper_ping", Arguments: `{"text":"yo"}`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res != "pong:yo" {
		t.Fatalf("registry output = %q", res)
	}
}

func TestBrokenServerDoesNotStopOthers(t *testing.T) {
	reg := tool.NewRegistry(nil)
	errs := Load(reg, map[string]ServerConfig{
		"broken": {Name: "broken", Command: "/nonexistent/definitely-not-a-binary"},
		"helper": helperConfig(),
	})
	if len(errs) == 0 {
		t.Fatal("expected a failure for the broken server")
	}
	found := false
	for _, n := range reg.Names() {
		if n == "mcp_helper_ping" {
			found = true
		}
	}
	if !found {
		t.Fatal("healthy server tools were not registered")
	}
}
