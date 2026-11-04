package obs

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestJSONLogHasFourFields(t *testing.T) {
	var buf bytes.Buffer
	Init(&buf, true)
	With("serve").Info("listening", "addr", "127.0.0.1:8787")

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("no log output")
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("not JSON: %q", line)
	}
	for _, k := range []string{"time", "level", "component", "msg"} {
		if _, ok := rec[k]; !ok {
			t.Errorf("missing %q in %v", k, rec)
		}
	}
	if rec["component"] != "serve" || rec["level"] != "INFO" {
		t.Fatalf("record = %v", rec)
	}
}

func TestMetricsRenderPrometheus(t *testing.T) {
	Reset()
	Inc("keel_http_requests_total")
	Add("keel_tokens_total", 42)
	Set("keel_tickets_open", func() int64 { return 3 })

	out := Render()
	for _, want := range []string{
		"# TYPE keel_http_requests_total counter",
		"keel_http_requests_total 1",
		"keel_tokens_total 42",
		"# TYPE keel_tickets_open gauge",
		"keel_tickets_open 3",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
