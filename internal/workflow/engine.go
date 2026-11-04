package workflow

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Deps are the side effects the engine needs from the host.
type Deps struct {
	// HTTP fetches a URL. Defaults to a real client.
	HTTP func(ctx context.Context, url string) (string, error)
	// Agent runs one prompt. Required when the workflow has agent nodes.
	Agent func(ctx context.Context, prompt string) (string, error)
}

// Run is one execution record.
type Run struct {
	ID       string    `json:"id"`
	Workflow string    `json:"workflow"`
	Status   string    `json:"status"` // passed | failed
	Started  int64     `json:"started_at"`
	Ended    int64     `json:"ended_at"`
	Nodes    []NodeRun `json:"nodes"`
}

// NodeRun is one node's outcome.
type NodeRun struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Status   string `json:"status"` // passed | failed
	Attempts int    `json:"attempts"`
	Started  int64  `json:"started_at"`
	Ended    int64  `json:"ended_at"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
}

var tmpl = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

// Execute runs a definition to completion. A failing node stops the run; the
// remaining nodes are not attempted.
func Execute(ctx context.Context, d *Definition, deps Deps) *Run {
	if deps.HTTP == nil {
		deps.HTTP = defaultHTTP
	}
	run := &Run{ID: runID(), Workflow: d.ID, Status: "passed", Started: time.Now().Unix()}
	outputs := map[string]string{}

	order, err := d.Order()
	if err != nil {
		run.Status = "failed"
		run.Ended = time.Now().Unix()
		run.Nodes = append(run.Nodes, NodeRun{Status: "failed", Error: err.Error()})
		return run
	}

	for _, id := range order {
		node := d.Node(id)
		nr := NodeRun{ID: node.ID, Type: node.Type, Started: time.Now().Unix()}
		out, attempts, err := runNode(ctx, *node, deps, outputs)
		nr.Attempts = attempts
		nr.Ended = time.Now().Unix()
		if err != nil {
			nr.Status = "failed"
			nr.Error = err.Error()
			run.Nodes = append(run.Nodes, nr)
			run.Status = "failed"
			run.Ended = time.Now().Unix()
			return run
		}
		nr.Status = "passed"
		nr.Output = out
		outputs[node.ID] = out
		run.Nodes = append(run.Nodes, nr)
	}
	run.Ended = time.Now().Unix()
	return run
}

func runNode(ctx context.Context, n Node, deps Deps, outputs map[string]string) (string, int, error) {
	attempts := 1
	backoff := 500 * time.Millisecond
	if n.Retry != nil {
		if n.Retry.Attempts > 0 {
			attempts = n.Retry.Attempts
		}
		backoff = parseDuration(n.Retry.Backoff, backoff)
	}
	timeout := parseDuration(n.Timeout, 30*time.Second)

	var lastErr error
	for i := 1; i <= attempts; i++ {
		actx, cancel := context.WithTimeout(ctx, timeout)
		out, err := execOnce(actx, n, deps, outputs)
		cancel()
		if err == nil {
			return out, i, nil
		}
		lastErr = err
		if i < attempts {
			select {
			case <-ctx.Done():
				return "", i, ctx.Err()
			case <-time.After(backoff * time.Duration(1<<(i-1))):
			}
		}
	}
	return "", attempts, lastErr
}

func execOnce(ctx context.Context, n Node, deps Deps, outputs map[string]string) (string, error) {
	switch n.Type {
	case NodeHTTP:
		if deps.HTTP == nil {
			return "", fmt.Errorf("node %s: no http dependency", n.ID)
		}
		return deps.HTTP(ctx, substitute(n.URL, outputs))
	case NodeAgent:
		if deps.Agent == nil {
			return "", fmt.Errorf("node %s: no agent dependency", n.ID)
		}
		return deps.Agent(ctx, substitute(n.Prompt, outputs))
	case NodeDelay:
		d := parseDuration(n.Duration, 0)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(d):
			return "waited " + d.String(), nil
		}
	default:
		return "", fmt.Errorf("node %s: unknown type %q", n.ID, n.Type)
	}
}

// substitute replaces {{node-id}} with that node's output.
func substitute(s string, outputs map[string]string) string {
	return tmpl.ReplaceAllStringFunc(s, func(m string) string {
		key := strings.TrimSpace(m[2 : len(m)-2])
		if v, ok := outputs[key]; ok {
			return v
		}
		return m
	})
}

func defaultHTTP(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "keel-workflow")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return string(raw), fmt.Errorf("http %d", resp.StatusCode)
	}
	return string(raw), nil
}

func runID() string {
	return fmt.Sprintf("run_%d", time.Now().UnixNano())
}
