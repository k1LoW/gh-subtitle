package translator

import (
	"os"
	"path/filepath"
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

func TestResolveCopilotToken(t *testing.T) {
	tests := []struct {
		name    string
		copilot string
		ghToken string
		github  string
		want    string
	}{
		{name: "COPILOT_GITHUB_TOKEN takes precedence", copilot: "copilot", ghToken: "gh", github: "github", want: "copilot"},
		{name: "GH_TOKEN over GITHUB_TOKEN", ghToken: "gh", github: "github", want: "gh"},
		{name: "GITHUB_TOKEN only", github: "github", want: "github"},
		{name: "no token falls back to the logged-in user", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COPILOT_GITHUB_TOKEN", tt.copilot)
			t.Setenv("GH_TOKEN", tt.ghToken)
			t.Setenv("GITHUB_TOKEN", tt.github)
			if got := resolveCopilotToken(); got != tt.want {
				t.Errorf("resolveCopilotToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveCopilotCLIPath(t *testing.T) {
	dir := t.TempDir()
	onPath := filepath.Join(dir, "copilot")
	if err := os.WriteFile(onPath, []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec // LookPath only finds executable files
		t.Fatal(err)
	}

	t.Run("COPILOT_CLI_PATH takes precedence over PATH", func(t *testing.T) {
		t.Setenv("PATH", dir)
		t.Setenv("COPILOT_CLI_PATH", "/opt/copilot/bin/copilot")
		got, err := resolveCopilotCLIPath()
		if err != nil {
			t.Fatal(err)
		}
		if want := "/opt/copilot/bin/copilot"; got != want {
			t.Errorf("resolveCopilotCLIPath() = %q, want %q", got, want)
		}
	})

	t.Run("copilot on PATH is used when COPILOT_CLI_PATH is unset", func(t *testing.T) {
		t.Setenv("PATH", dir)
		t.Setenv("COPILOT_CLI_PATH", "")
		got, err := resolveCopilotCLIPath()
		if err != nil {
			t.Fatal(err)
		}
		if got != onPath {
			t.Errorf("resolveCopilotCLIPath() = %q, want %q", got, onPath)
		}
	})

	t.Run("error when copilot is not found", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		t.Setenv("COPILOT_CLI_PATH", "")
		if _, err := resolveCopilotCLIPath(); err == nil {
			t.Error("resolveCopilotCLIPath() returned no error")
		}
	})
}
