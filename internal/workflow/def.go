// Package workflow runs small configuration-defined DAGs.
//
// The point is not to clone n8n. It is to cover the shapes an agent project
// actually needs: fetch something, think about it, wait, hand the result on —
// with retries, timeouts and a record of what happened.
package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Node types.
const (
	NodeHTTP  = "http"
	NodeAgent = "agent"
	NodeDelay = "delay"
)

// Retry configures one node's attempts.
type Retry struct {
	Attempts int    `json:"attempts,omitempty"`
	Backoff  string `json:"backoff,omitempty"`
}

// Node is one step.
type Node struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Depends  []string `json:"depends,omitempty"`
	URL      string   `json:"url,omitempty"`
	Prompt   string   `json:"prompt,omitempty"`
	Duration string   `json:"duration,omitempty"`
	Timeout  string   `json:"timeout,omitempty"`
	Retry    *Retry   `json:"retry,omitempty"`
}

// Definition is one workflow file.
type Definition struct {
	ID    string `json:"id"`
	Nodes []Node `json:"nodes"`
}

// Load reads and validates a workflow file.
func Load(path string) (*Definition, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d Definition
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, fmt.Errorf("workflow %s: %w", path, err)
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	return &d, nil
}

// Validate rejects duplicate ids, unknown dependencies and cycles before any
// work starts.
func (d *Definition) Validate() error {
	if len(d.Nodes) == 0 {
		return fmt.Errorf("workflow: no nodes")
	}
	ids := map[string]bool{}
	for _, n := range d.Nodes {
		if n.ID == "" {
			return fmt.Errorf("workflow: node without id")
		}
		if ids[n.ID] {
			return fmt.Errorf("workflow: duplicate node id %q", n.ID)
		}
		ids[n.ID] = true
		switch n.Type {
		case NodeHTTP, NodeAgent, NodeDelay:
		default:
			return fmt.Errorf("workflow: node %q has unknown type %q", n.ID, n.Type)
		}
	}
	for _, n := range d.Nodes {
		for _, dep := range n.Depends {
			if !ids[dep] {
				return fmt.Errorf("workflow: node %q depends on missing %q", n.ID, dep)
			}
			if dep == n.ID {
				return fmt.Errorf("workflow: node %q depends on itself", n.ID)
			}
		}
	}
	if _, err := d.Order(); err != nil {
		return err
	}
	return nil
}

// Order returns node ids in dependency order (Kahn).
func (d *Definition) Order() ([]string, error) {
	indeg := map[string]int{}
	next := map[string][]string{}
	for _, n := range d.Nodes {
		indeg[n.ID] = len(n.Depends)
		for _, dep := range n.Depends {
			next[dep] = append(next[dep], n.ID)
		}
	}
	var queue []string
	for _, n := range d.Nodes {
		if indeg[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}
	var out []string
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		out = append(out, id)
		for _, ch := range next[id] {
			indeg[ch]--
			if indeg[ch] == 0 {
				queue = append(queue, ch)
			}
		}
	}
	if len(out) != len(d.Nodes) {
		return nil, fmt.Errorf("workflow: dependency cycle")
	}
	return out, nil
}

// Node returns one node by id.
func (d *Definition) Node(id string) *Node {
	for i := range d.Nodes {
		if d.Nodes[i].ID == id {
			return &d.Nodes[i]
		}
	}
	return nil
}

func parseDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
