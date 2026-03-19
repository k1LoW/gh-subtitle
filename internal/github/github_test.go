package github

import (
	"testing"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    *ParsedURL
		wantErr bool
	}{
		{
			name: "PR URL",
			url:  "https://github.com/owner/repo/pull/123",
			want: &ParsedURL{Owner: "owner", Repo: "repo", Type: "pull", Number: 123},
		},
		{
			name: "Issue URL",
			url:  "https://github.com/owner/repo/issues/456",
			want: &ParsedURL{Owner: "owner", Repo: "repo", Type: "issues", Number: 456},
		},
		{
			name: "Discussion URL",
			url:  "https://github.com/owner/repo/discussions/789",
			want: &ParsedURL{Owner: "owner", Repo: "repo", Type: "discussions", Number: 789},
		},
		{
			name:    "invalid URL - too short",
			url:     "https://github.com/owner/repo",
			wantErr: true,
		},
		{
			name:    "invalid URL - bad number",
			url:     "https://github.com/owner/repo/pull/abc",
			wantErr: true,
		},
		{
			name:    "unsupported type",
			url:     "https://github.com/owner/repo/wiki/123",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil {
				if got.Owner != tt.want.Owner || got.Repo != tt.want.Repo || got.Type != tt.want.Type || got.Number != tt.want.Number {
					t.Errorf("ParseURL() = %+v, want %+v", got, tt.want)
				}
			}
		})
	}
}
