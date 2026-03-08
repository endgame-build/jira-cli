package markdown

import (
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
)

func TestIssuePath(t *testing.T) {
	tests := []struct {
		name  string
		issue api.Issue
		want  string
	}{
		{
			name: "basic issue with project",
			issue: api.Issue{
				Key: "PROJ-123",
				Fields: api.IssueFields{
					Summary: "Fix login bug",
					Project: &api.Project{Key: "PROJ"},
				},
			},
			want: "PROJ/PROJ-123 - Fix login bug.md",
		},
		{
			name: "nil project falls back to key prefix",
			issue: api.Issue{
				Key: "MYAPP-42",
				Fields: api.IssueFields{
					Summary: "Add feature",
				},
			},
			want: "MYAPP/MYAPP-42 - Add feature.md",
		},
		{
			name: "special chars in summary sanitized",
			issue: api.Issue{
				Key: "PROJ-1",
				Fields: api.IssueFields{
					Summary: "Fix: path/to\\file <test> \"quoted\" | pipe",
					Project: &api.Project{Key: "PROJ"},
				},
			},
			want: "PROJ/PROJ-1 - Fix- path-to-file -test- -quoted- - pipe.md",
		},
		{
			name: "empty summary",
			issue: api.Issue{
				Key: "PROJ-5",
				Fields: api.IssueFields{
					Summary: "",
					Project: &api.Project{Key: "PROJ"},
				},
			},
			want: "PROJ/PROJ-5 - .md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IssuePath(tt.issue)
			if got != tt.want {
				t.Errorf("IssuePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no special chars",
			input: "simple name",
			want:  "simple name",
		},
		{
			name:  "slash replaced",
			input: "path/to/file",
			want:  "path-to-file",
		},
		{
			name:  "backslash replaced",
			input: "path\\to\\file",
			want:  "path-to-file",
		},
		{
			name:  "colon replaced",
			input: "title: subtitle",
			want:  "title- subtitle",
		},
		{
			name:  "asterisk replaced",
			input: "important*item",
			want:  "important-item",
		},
		{
			name:  "question mark replaced",
			input: "is this ok?",
			want:  "is this ok-",
		},
		{
			name:  "quotes replaced",
			input: `say "hello"`,
			want:  "say -hello-",
		},
		{
			name:  "angle brackets replaced",
			input: "a<b>c",
			want:  "a-b-c",
		},
		{
			name:  "pipe replaced",
			input: "a|b",
			want:  "a-b",
		},
		{
			name:  "multiple spaces collapsed",
			input: "too   many    spaces",
			want:  "too many spaces",
		},
		{
			name:  "leading and trailing whitespace trimmed",
			input: "  hello  ",
			want:  "hello",
		},
		{
			name:  "long name truncated to 100 chars",
			input: repeat("a", 150),
			want:  repeat("a", 100),
		},
		{
			name:  "exactly 100 chars kept",
			input: repeat("x", 100),
			want:  repeat("x", 100),
		},
		{
			name:  "101 chars truncated",
			input: repeat("x", 101),
			want:  repeat("x", 100),
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "unicode preserved",
			input: "日本語テスト",
			want:  "日本語テスト",
		},
		{
			name:  "all special chars",
			input: `/\:*?"<>|`,
			want:  "---------",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeFilenameLongTruncation(t *testing.T) {
	// Verify trailing whitespace is trimmed after truncation
	input := repeat("a", 99) + " b"
	got := SanitizeFilename(input)
	if len(got) > 100 {
		t.Errorf("SanitizeFilename should truncate to max 100 chars, got %d", len(got))
	}
	// 99 'a' + " b" = 101 → truncated to 100 → "aaa...a " → TrimSpace → 99 'a's
	if want := repeat("a", 99); got != want {
		t.Errorf("SanitizeFilename(%q) = %q, want %q", input, got, want)
	}
}

func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
