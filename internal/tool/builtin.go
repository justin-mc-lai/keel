package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/justin-mc-lai/agentdesk/internal/workspace"
)

const (
	maxReadBytes  = 256 * 1024
	maxFetchBytes = 100 * 1024
	maxListItems  = 500
	maxGrepHits   = 100
	shellLimit    = 30 * time.Second
)

// Builtins returns the default tool set. File tools refuse any path outside
// the workspace; write-side tools ask for approval.
func Builtins(ws *workspace.Workspace) []Tool {
	resolve := func(p string) (string, error) {
		if ws == nil {
			return "", fmt.Errorf("workspace not configured")
		}
		return ws.Resolve(p)
	}

	return []Tool{
		{
			Name:        "read_file",
			Description: "Read a UTF-8 text file inside the workspace.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
			Run: func(ctx context.Context, args json.RawMessage) (string, error) {
				var in struct {
					Path string `json:"path"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return "", err
				}
				p, err := resolve(in.Path)
				if err != nil {
					return "", err
				}
				raw, err := os.ReadFile(p)
				if err != nil {
					return "", err
				}
				if len(raw) > maxReadBytes {
					return string(raw[:maxReadBytes]) + "\n[truncated]", nil
				}
				return string(raw), nil
			},
		},
		{
			Name:             "write_file",
			Description:      "Create or overwrite a text file inside the workspace. Requires approval.",
			Parameters:       json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`),
			RequiresApproval: true,
			Run: func(ctx context.Context, args json.RawMessage) (string, error) {
				var in struct {
					Path    string `json:"path"`
					Content string `json:"content"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return "", err
				}
				p, err := resolve(in.Path)
				if err != nil {
					return "", err
				}
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					return "", err
				}
				if err := os.WriteFile(p, []byte(in.Content), 0o644); err != nil {
					return "", err
				}
				return fmt.Sprintf("wrote %d bytes to %s", len(in.Content), in.Path), nil
			},
		},
		{
			Name:             "edit_file",
			Description:      "Replace exactly one occurrence of old with new in a workspace file. Requires approval.",
			Parameters:       json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"old":{"type":"string"},"new":{"type":"string"}},"required":["path","old","new"]}`),
			RequiresApproval: true,
			Run: func(ctx context.Context, args json.RawMessage) (string, error) {
				var in struct {
					Path string `json:"path"`
					Old  string `json:"old"`
					New  string `json:"new"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return "", err
				}
				p, err := resolve(in.Path)
				if err != nil {
					return "", err
				}
				raw, err := os.ReadFile(p)
				if err != nil {
					return "", err
				}
				if strings.Count(string(raw), in.Old) != 1 {
					return "", fmt.Errorf("edit_file: %q must appear exactly once in %s", in.Old, in.Path)
				}
				next := strings.Replace(string(raw), in.Old, in.New, 1)
				if err := os.WriteFile(p, []byte(next), 0o644); err != nil {
					return "", err
				}
				return fmt.Sprintf("edited %s", in.Path), nil
			},
		},
		{
			Name:        "list_dir",
			Description: "List directory entries inside the workspace. Directories end with /.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
			Run: func(ctx context.Context, args json.RawMessage) (string, error) {
				var in struct {
					Path string `json:"path"`
				}
				_ = json.Unmarshal(args, &in)
				p, err := resolve(in.Path)
				if err != nil {
					return "", err
				}
				entries, err := os.ReadDir(p)
				if err != nil {
					return "", err
				}
				names := make([]string, 0, len(entries))
				for _, e := range entries {
					n := e.Name()
					if e.IsDir() {
						n += "/"
					}
					names = append(names, n)
					if len(names) >= maxListItems {
						break
					}
				}
				sort.Strings(names)
				return strings.Join(names, "\n"), nil
			},
		},
		{
			Name:        "grep",
			Description: "Search workspace files for a regular expression. Returns path:line:text.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"}},"required":["pattern"]}`),
			Run: func(ctx context.Context, args json.RawMessage) (string, error) {
				var in struct {
					Pattern string `json:"pattern"`
					Path    string `json:"path"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return "", err
				}
				re, err := regexp.Compile(in.Pattern)
				if err != nil {
					return "", err
				}
				root, err := resolve(in.Path)
				if err != nil {
					return "", err
				}
				var hits []string
				walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
					if err != nil || len(hits) >= maxGrepHits {
						return nil
					}
					if info.IsDir() {
						if info.Name() == ".git" || info.Name() == "node_modules" {
							return filepath.SkipDir
						}
						return nil
					}
					if info.Size() > 1024*1024 {
						return nil
					}
					raw, err := os.ReadFile(path)
					if err != nil || bytes.IndexByte(raw, 0) >= 0 {
						return nil
					}
					rel, _ := filepath.Rel(ws.Root(), path)
					for i, line := range strings.Split(string(raw), "\n") {
						if re.MatchString(line) {
							hits = append(hits, fmt.Sprintf("%s:%d:%s", rel, i+1, strings.TrimSpace(line)))
							if len(hits) >= maxGrepHits {
								return nil
							}
						}
					}
					return nil
				})
				if walkErr != nil {
					return "", walkErr
				}
				return strings.Join(hits, "\n"), nil
			},
		},
		{
			Name:        "fetch_url",
			Description: "Fetch a URL and return its body as text (first 100 KB).",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}`),
			Run: func(ctx context.Context, args json.RawMessage) (string, error) {
				var in struct {
					URL string `json:"url"`
				}
				if err := json.Unmarshal(args, &in); err != nil {
					return "", err
				}
				if !strings.HasPrefix(in.URL, "http://") && !strings.HasPrefix(in.URL, "https://") {
					return "", fmt.Errorf("fetch_url: only http and https are supported")
				}
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.URL, nil)
				if err != nil {
					return "", err
				}
				req.Header.Set("User-Agent", "agentdesk/"+Version)
				resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
				if err != nil {
					return "", err
				}
				defer resp.Body.Close()
				raw, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes))
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("HTTP %d\n\n%s", resp.StatusCode, string(raw)), nil
			},
		},
		{
			Name:             "run_shell",
			Description:      "Run a shell command in the workspace root and return combined output. Requires approval.",
			Parameters:       json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`),
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
				cctx, cancel := context.WithTimeout(ctx, shellLimit)
				defer cancel()
				cmd := exec.CommandContext(cctx, "sh", "-c", in.Command)
				if ws != nil {
					cmd.Dir = ws.Root()
				}
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
			Name:        "get_time",
			Description: "Return the current time in an IANA timezone. Defaults to UTC.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"tz":{"type":"string"}}}`),
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

// Version is stamped into the fetch user agent.
const Version = "0.2.0"

// NewDefaultRegistry registers the built-ins and applies the approval policy.
func NewDefaultRegistry(approve Approver, ws *workspace.Workspace) (*Registry, error) {
	r := NewRegistry(approve)
	for _, t := range Builtins(ws) {
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
