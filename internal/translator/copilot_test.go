package translator

import "testing"

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
