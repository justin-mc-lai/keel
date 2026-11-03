package tool

import (
	"context"
	"encoding/json"
	"fmt"
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
