package translator

import (
	"testing"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

func TestStripCodeFence(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no fence",
			in:   `[{"key":"body"}]`,
			want: `[{"key":"body"}]`,
		},
		{
			name: "json fence",
			in:   "```json\n[{\"key\":\"body\"}]\n```",
			want: `[{"key":"body"}]`,
		},
		{
			name: "bare fence",
			in:   "```\n[{\"key\":\"body\"}]\n```",
			want: `[{"key":"body"}]`,
		},
		{
			name: "fence with surrounding whitespace",
			in:   "\n  ```json\n[{\"key\":\"body\"}]\n```\n",
			want: `[{"key":"body"}]`,
		},
		{
			name: "multi-line content inside fence",
			in:   "```json\n[\n  {\"key\":\"body\"}\n]\n```",
			want: "[\n  {\"key\":\"body\"}\n]",
		},
		{
			name: "fence inside translated text is kept",
			in:   "[{\"key\":\"body\",\"text\":\"```go\\nfmt.Println()\\n```\"}]",
			want: "[{\"key\":\"body\",\"text\":\"```go\\nfmt.Println()\\n```\"}]",
		},
		{
			name: "opening fence only",
			in:   "```json\n[{\"key\":\"body\"}]",
			want: "```json\n[{\"key\":\"body\"}]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripCodeFence(tt.in); got != tt.want {
				t.Errorf("stripCodeFence(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNewSessionConfigDisablesTools(t *testing.T) {
	cfg := newSessionConfig("", nil, "prompt")
	if cfg.AvailableTools == nil {
		t.Fatal("AvailableTools is nil, which falls back to the default tool set")
	}
	if len(cfg.AvailableTools) != 0 {
		t.Errorf("AvailableTools = %v, want empty", cfg.AvailableTools)
	}
}

func TestNewSessionConfigDeniesPermissions(t *testing.T) {
	cfg := newSessionConfig("", nil, "prompt")
	if cfg.OnPermissionRequest == nil {
		t.Fatal("OnPermissionRequest is nil")
	}
	requests := []copilot.PermissionRequest{
		&rpc.PermissionRequestShell{},
		&rpc.PermissionRequestWrite{},
		&rpc.PermissionRequestURL{},
		&rpc.PermissionRequestMCP{},
	}
	for _, req := range requests {
		decision, err := cfg.OnPermissionRequest(req, copilot.PermissionInvocation{})
		if err != nil {
			t.Fatalf("%T: unexpected error: %v", req, err)
		}
		if _, ok := decision.(*rpc.PermissionDecisionReject); !ok {
			t.Errorf("%T: got %T, want *rpc.PermissionDecisionReject", req, decision)
		}
	}
}
