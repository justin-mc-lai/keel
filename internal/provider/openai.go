package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// OpenAICompatible speaks the /chat/completions wire format that basically
// every gateway implements. One implementation, many vendors.
type OpenAICompatible struct {
	name          string
	baseURL       string
	apiKey        string
	headers       map[string]string
	sessionHeader string
	models        []string
	defaultModel  string
	client        *http.Client
	attempts      int
}

// OpenAICompatibleOptions configures one vendor endpoint.
type OpenAICompatibleOptions struct {
	Name          string
	BaseURL       string
	APIKey        string
	Headers       map[string]string
	SessionHeader string
	Models        []string
	DefaultModel  string
	// NoProxy forces a direct connection. Command Code resets long streams
	// when they go through a local proxy, so it must bypass it.
	NoProxy bool
	Timeout time.Duration
}

// NewOpenAICompatible builds a provider. BaseURL is the API root that already
// includes the vendor path, e.g. https://opencode.ai/zen/go/v1.
func NewOpenAICompatible(o OpenAICompatibleOptions) *OpenAICompatible {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if o.NoProxy {
		tr.Proxy = nil
	}
	timeout := o.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	return &OpenAICompatible{
		name:          o.Name,
		baseURL:       strings.TrimRight(o.BaseURL, "/"),
		apiKey:        o.APIKey,
		headers:       o.Headers,
		sessionHeader: o.SessionHeader,
		models:        o.Models,
		defaultModel:  o.DefaultModel,
		client:        &http.Client{Transport: tr, Timeout: timeout},
		attempts:      3,
	}
}

func (p *OpenAICompatible) Name() string         { return p.name }
func (p *OpenAICompatible) DefaultModel() string { return p.defaultModel }
func (p *OpenAICompatible) Models() []string     { return append([]string(nil), p.models...) }

func (p *OpenAICompatible) Stream(ctx context.Context, req Request) (<-chan Event, error) {
	model := req.Model
	if model == "" {
		model = p.defaultModel
	}
	body, err := json.Marshal(p.wireRequest(model, req))
	if err != nil {
		return nil, fmt.Errorf("provider %s: encode request: %w", p.name, err)
	}

	// Some gateways drop a connection before the response headers on the first
	// attempt (observed ~1 in 3 against api.commandcode.ai). Retrying here is
	// safe because nothing has been consumed yet; once streaming starts we can
	// never replay. Body is a bytes.Reader, so each attempt gets a fresh one.
	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt < p.attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 400 * time.Millisecond):
			}
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "text/event-stream")
		if p.apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		}
		for k, v := range p.headers {
			httpReq.Header.Set(k, v)
		}
		if p.sessionHeader != "" && req.SessionID != "" {
			httpReq.Header.Set(p.sessionHeader, req.SessionID)
		}

		resp, err = p.client.Do(httpReq)
		if err == nil {
			lastErr = nil
			break
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, fmt.Errorf("provider %s: %w", p.name, lastErr)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("provider %s: HTTP %d: %s", p.name, resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	ch := make(chan Event, 128)
	go p.pump(ctx, resp.Body, ch)
	return ch, nil
}

func (p *OpenAICompatible) wireRequest(model string, req Request) map[string]any {
	msgs := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		item := map[string]any{"role": m.Role, "content": m.Content}
		if m.Role == RoleTool {
			item["tool_call_id"] = m.ToolCallID
			if m.Name != "" {
				item["name"] = m.Name
			}
		}
		if len(m.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				calls = append(calls, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				})
			}
			item["tool_calls"] = calls
			item["content"] = nil
		}
		msgs = append(msgs, item)
	}
	out := map[string]any{
		"model":    model,
		"messages": msgs,
		"stream":   true,
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			})
		}
		out["tools"] = tools
	}
	if req.MaxTokens > 0 {
		out["max_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		out["temperature"] = *req.Temperature
	}
	if req.Effort != "" {
		out["reasoning_effort"] = req.Effort
	}
	return out
}

// chunk mirrors one SSE data payload of the OpenAI streaming format.
type chunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// pump parses the SSE stream and closes ch when the body ends.
func (p *OpenAICompatible) pump(ctx context.Context, body io.ReadCloser, ch chan<- Event) {
	defer close(ch)
	defer body.Close()

	type partial struct {
		id      string
		name    string
		args    strings.Builder
		hasData bool
	}
	pending := map[int]*partial{}
	var usage *Usage

	emit := func(ev Event) bool {
		select {
		case ch <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}

	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		if payload == "[DONE]" {
			break
		}
		var c chunk
		if err := json.Unmarshal([]byte(payload), &c); err != nil {
			continue
		}
		if c.Error != nil && c.Error.Message != "" {
			emit(Event{Type: EventError, Err: fmt.Errorf("provider %s: %s", p.name, c.Error.Message)})
			return
		}
		if c.Usage != nil {
			usage = c.Usage
		}
		for _, choice := range c.Choices {
			d := choice.Delta
			if t := firstNonEmpty(d.ReasoningContent, d.Reasoning); t != "" {
				if !emit(Event{Type: EventReasoning, Text: t}) {
					return
				}
			}
			if d.Content != "" {
				if !emit(Event{Type: EventContent, Text: d.Content}) {
					return
				}
			}
			for _, tc := range d.ToolCalls {
				cur := pending[tc.Index]
				if cur == nil {
					cur = &partial{}
					pending[tc.Index] = cur
				}
				if tc.ID != "" {
					cur.id = tc.ID
				}
				if tc.Function.Name != "" {
					cur.name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					cur.args.WriteString(tc.Function.Arguments)
					cur.hasData = true
				}
			}
		}
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		emit(Event{Type: EventError, Err: fmt.Errorf("provider %s: read stream: %w", p.name, err)})
		return
	}

	indexes := make([]int, 0, len(pending))
	for idx := range pending {
		indexes = append(indexes, idx)
	}
	sort.Ints(indexes)
	for _, idx := range indexes {
		pp := pending[idx]
		if pp.name == "" {
			continue
		}
		args := pp.args.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		call := ToolCall{ID: pp.id, Name: pp.name, Arguments: args}
		if call.ID == "" {
			call.ID = fmt.Sprintf("call_%s_%d", p.name, idx)
		}
		if !emit(Event{Type: EventToolCall, ToolCall: &call}) {
			return
		}
	}
	if usage != nil {
		emit(Event{Type: EventUsage, Usage: usage})
	}
	emit(Event{Type: EventDone})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
