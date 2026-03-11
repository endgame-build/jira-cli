package markdown

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
)

func fullIssue() api.Issue {
	return api.Issue{
		ID:  "10001",
		Key: "PROJ-123",
		Fields: api.IssueFields{
			Summary:     "Fix login bug",
			Description: json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"Description text"}]}]}`),
			IssueType:   &api.IssueType{Name: "Bug"},
			Status:      &api.Status{Name: "In Progress"},
			Priority:    &api.Priority{Name: "High"},
			Project:     &api.Project{Key: "PROJ"},
			Parent:      &api.IssueParent{Key: "PROJ-100"},
			Assignee:    &api.User{DisplayName: "Alice Smith", AccountID: "abc123"},
			Reporter:    &api.User{DisplayName: "Bob Jones", AccountID: "def456"},
			Labels:      []string{"backend", "urgent"},
			Created:     "2026-01-15T10:00:00.000+0000",
			Updated:     "2026-02-20T14:30:00.000+0000",
		},
	}
}

func TestIssueToMarkdown(t *testing.T) {
	tests := []struct {
		name       string
		issue      api.Issue
		wantParts  []string // substrings that must appear in output
		wantAbsent []string // substrings that must NOT appear
	}{
		{
			name:  "full issue with all fields",
			issue: fullIssue(),
			wantParts: []string{
				"---\n",
				"key: PROJ-123",
				"id: \"10001\"",
				"type: Bug",
				"summary: Fix login bug",
				"status: In Progress",
				"priority: High",
				"parent: PROJ-100",
				"assignee: Alice Smith",
				"assignee_id: abc123",
				"reporter: Bob Jones",
				"reporter_id: def456",
				"project: PROJ",
				"created: 2026-01-15T10:00:00.000+0000",
				"updated: 2026-02-20T14:30:00.000+0000",
				"labels:\n    - backend\n    - urgent",
				"Description text",
			},
		},
		{
			name: "nil optional fields omitted",
			issue: api.Issue{
				ID:  "10002",
				Key: "TEST-1",
				Fields: api.IssueFields{
					Summary: "Simple issue",
				},
			},
			wantParts: []string{
				"key: TEST-1",
				"summary: Simple issue",
			},
			wantAbsent: []string{
				"type:",
				"status:",
				"priority:",
				"parent:",
				"assignee:",
				"assignee_id:",
				"reporter:",
				"reporter_id:",
				"project:",
				"labels:",
			},
		},
		{
			name: "nil description produces no body",
			issue: api.Issue{
				Key: "TEST-2",
				Fields: api.IssueFields{
					Summary: "No description",
				},
			},
			wantParts: []string{
				"key: TEST-2",
				"summary: No description",
			},
		},
		{
			name: "empty labels omitted",
			issue: api.Issue{
				Key: "TEST-3",
				Fields: api.IssueFields{
					Summary: "Empty labels",
					Labels:  []string{},
				},
			},
			wantAbsent: []string{"labels:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IssueToMarkdown(tt.issue, nil, nil)
			if err != nil {
				t.Fatalf("IssueToMarkdown() error = %v", err)
			}

			output := string(got)

			// Must start and end with frontmatter delimiters
			if !strings.HasPrefix(output, "---\n") {
				t.Error("output should start with ---")
			}

			for _, part := range tt.wantParts {
				if !strings.Contains(output, part) {
					t.Errorf("output missing expected substring %q\ngot:\n%s", part, output)
				}
			}

			for _, absent := range tt.wantAbsent {
				if strings.Contains(output, absent) {
					t.Errorf("output should not contain %q\ngot:\n%s", absent, output)
				}
			}
		})
	}
}

func TestIssueToMarkdownStructure(t *testing.T) {
	issue := fullIssue()
	got, err := IssueToMarkdown(issue, nil, nil)
	if err != nil {
		t.Fatalf("IssueToMarkdown() error = %v", err)
	}

	output := string(got)

	// Verify structure: frontmatter between --- delimiters, then body
	parts := strings.SplitN(output, "---\n", 3)
	if len(parts) < 3 {
		t.Fatalf("expected at least 3 parts split on ---, got %d", len(parts))
	}

	// parts[0] should be empty (before first ---)
	if parts[0] != "" {
		t.Errorf("expected empty string before first ---, got %q", parts[0])
	}

	// parts[1] should be the YAML frontmatter
	if !strings.Contains(parts[1], "key: PROJ-123") {
		t.Error("frontmatter section should contain key")
	}

	// parts[2] should be the markdown body
	if !strings.Contains(parts[2], "Description text") {
		t.Error("body section should contain description text")
	}
}
