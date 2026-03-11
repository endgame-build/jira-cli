package markdown

import (
	"bytes"
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

func TestNormalizeFieldName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Story Points", "story_points"},
		{"Team", "team"},
		{"Custom Field (v2)", "custom_field_v2"},
		{"!!!", ""},
		{"Already_Normal", "already_normal"},
		{"  spaces  ", "__spaces__"},
		{"MixedCase123", "mixedcase123"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeFieldName(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeFieldName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsBuiltinKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"key", true},
		{"summary", true},
		{"status", true},
		{"assignee_id", true},
		{"team", false},
		{"story_points", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := IsBuiltinKey(tt.key)
			if got != tt.want {
				t.Errorf("IsBuiltinKey(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestExtractCustomFieldValue(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantVal interface{}
		wantOK  bool
	}{
		{"string", `"hello"`, "hello", true},
		{"number", `42.5`, 42.5, true},
		{"bool", `true`, true, true},
		{"null", `null`, nil, false},
		{"object with value string", `{"value":"Critical"}`, "Critical", true},
		{"object with value number", `{"value":10}`, float64(10), true},
		{"object with name", `{"name":"Platform","id":"123"}`, "Platform", true},
		{"object with neither", `{"id":"123","foo":"bar"}`, nil, false},
		{"array", `["a","b"]`, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, ok := extractCustomFieldValue(json.RawMessage(tt.raw))
			if ok != tt.wantOK {
				t.Errorf("extractCustomFieldValue(%s) ok = %v, want %v", tt.raw, ok, tt.wantOK)
			}
			if ok && val != tt.wantVal {
				t.Errorf("extractCustomFieldValue(%s) val = %v (%T), want %v (%T)", tt.raw, val, val, tt.wantVal, tt.wantVal)
			}
		})
	}
}

func TestIssueToMarkdownWithCustomFields(t *testing.T) {
	issue := api.Issue{
		Key: "PROJ-1",
		Fields: api.IssueFields{
			Summary: "Test custom fields",
			CustomFields: map[string]json.RawMessage{
				"customfield_10001": json.RawMessage(`"Platform"`),
				"customfield_10002": json.RawMessage(`5.0`),
				"customfield_10003": json.RawMessage(`{"value":"Critical"}`),
			},
		},
	}

	fields := map[string]api.Field{
		"customfield_10001": {ID: "customfield_10001", Name: "Team", Custom: true},
		"customfield_10002": {ID: "customfield_10002", Name: "Story Points", Custom: true},
		"customfield_10003": {ID: "customfield_10003", Name: "Severity", Custom: true},
	}

	got, err := IssueToMarkdown(issue, fields, nil)
	if err != nil {
		t.Fatalf("IssueToMarkdown() error = %v", err)
	}

	output := string(got)

	// Custom fields should appear after built-in fields
	for _, want := range []string{"team: Platform", "story_points: 5", "severity: Critical"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, output)
		}
	}

	// Built-in fields should still be present
	if !strings.Contains(output, "key: PROJ-1") {
		t.Errorf("output missing built-in key field\ngot:\n%s", output)
	}
}

func TestIssueToMarkdownBuiltinCollision(t *testing.T) {
	issue := api.Issue{
		Key: "PROJ-1",
		Fields: api.IssueFields{
			Summary: "Builtin collision test",
			Status:  &api.Status{Name: "Open"},
			CustomFields: map[string]json.RawMessage{
				"customfield_10001": json.RawMessage(`"custom status value"`),
			},
		},
	}

	fields := map[string]api.Field{
		"customfield_10001": {ID: "customfield_10001", Name: "Status", Custom: true},
	}

	var warnings bytes.Buffer
	got, err := IssueToMarkdown(issue, fields, &warnings)
	if err != nil {
		t.Fatalf("IssueToMarkdown() error = %v", err)
	}

	output := string(got)

	// Built-in status should be present
	if !strings.Contains(output, "status: Open") {
		t.Errorf("output missing built-in status\ngot:\n%s", output)
	}

	// Custom field value should NOT appear
	if strings.Contains(output, "custom status value") {
		t.Errorf("output should not contain custom field that collides with built-in\ngot:\n%s", output)
	}

	// Warning should be written
	if !strings.Contains(warnings.String(), "collides with built-in key") {
		t.Errorf("expected collision warning, got: %q", warnings.String())
	}
}

func TestIssueToMarkdownNilFields(t *testing.T) {
	issue := api.Issue{
		Key: "PROJ-1",
		Fields: api.IssueFields{
			Summary: "No custom fields",
			CustomFields: map[string]json.RawMessage{
				"customfield_10001": json.RawMessage(`"value"`),
			},
		},
	}

	// nil fields map = no custom field processing (backward compat)
	got, err := IssueToMarkdown(issue, nil, nil)
	if err != nil {
		t.Fatalf("IssueToMarkdown() error = %v", err)
	}

	output := string(got)
	if !strings.Contains(output, "key: PROJ-1") {
		t.Errorf("output missing key\ngot:\n%s", output)
	}

	// No custom fields should appear
	if strings.Contains(output, "customfield") || strings.Contains(output, "value") {
		// "value" could be in the raw field, but since fields=nil, nothing custom should appear.
		// Actually "value" might appear in YAML literally — let's check more specifically.
	}

	// The output should only have built-in fields
	parts := strings.SplitN(output, "---\n", 3)
	if len(parts) < 3 {
		t.Fatalf("expected frontmatter structure, got %d parts", len(parts))
	}
	yaml := parts[1]
	lines := strings.Split(strings.TrimSpace(yaml), "\n")
	for _, line := range lines {
		key := strings.SplitN(line, ":", 2)[0]
		key = strings.TrimSpace(key)
		if key != "" && !IsBuiltinKey(key) {
			t.Errorf("unexpected non-builtin key %q in output with nil fields map", key)
		}
	}
}

func TestIssueToMarkdownWarnWriter(t *testing.T) {
	issue := api.Issue{
		Key: "PROJ-1",
		Fields: api.IssueFields{
			Summary: "Warn test",
			CustomFields: map[string]json.RawMessage{
				"customfield_10001": json.RawMessage(`["array","value"]`),
				"customfield_10002": json.RawMessage(`"good"`),
				"customfield_10003": json.RawMessage(`"collision"`),
			},
		},
	}

	fields := map[string]api.Field{
		"customfield_10001": {ID: "customfield_10001", Name: "Tags", Custom: true},
		"customfield_10002": {ID: "customfield_10002", Name: "Team", Custom: true},
		"customfield_10003": {ID: "customfield_10003", Name: "Status", Custom: true}, // built-in collision
	}

	// Test with warnWriter
	var warnings bytes.Buffer
	got, err := IssueToMarkdown(issue, fields, &warnings)
	if err != nil {
		t.Fatalf("IssueToMarkdown() error = %v", err)
	}

	output := string(got)
	warnStr := warnings.String()

	// Array value should be skipped with warning
	if !strings.Contains(warnStr, "customfield_10001") {
		t.Errorf("expected warning for array field, got: %q", warnStr)
	}

	// Built-in collision should warn
	if !strings.Contains(warnStr, "collides with built-in") {
		t.Errorf("expected builtin collision warning, got: %q", warnStr)
	}

	// Good field should be in output
	if !strings.Contains(output, "team: good") {
		t.Errorf("output missing valid custom field\ngot:\n%s", output)
	}

	// Test with nil warnWriter — should not panic
	_, err = IssueToMarkdown(issue, fields, nil)
	if err != nil {
		t.Fatalf("IssueToMarkdown() with nil warnWriter error = %v", err)
	}
}
