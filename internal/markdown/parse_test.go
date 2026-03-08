package markdown

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	clierrors "github.com/endgame-build/jira-cli/internal/errors"
)

func writeTestFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const validFile = `---
key: PROJ-123
summary: Fix login bug
type: Bug
status: In Progress
priority: High
project: PROJ
assignee: Alice
assignee_id: abc123
labels:
    - backend
    - urgent
---
This is the description body.

It has multiple paragraphs.
`

func TestParseFile(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantKey  string
		wantBody string
		wantErr  bool
		errCode  clierrors.ErrorCode
	}{
		{
			name:     "valid file",
			content:  validFile,
			wantKey:  "PROJ-123",
			wantBody: "This is the description body.\n\nIt has multiple paragraphs.",
		},
		{
			name: "empty body allowed",
			content: `---
key: PROJ-456
summary: No description
---
`,
			wantKey:  "PROJ-456",
			wantBody: "",
		},
		{
			name:     "no trailing newline after closing delimiter",
			content:  "---\nkey: PROJ-789\nsummary: Test\n---",
			wantKey:  "PROJ-789",
			wantBody: "",
		},
		{
			name:    "missing frontmatter delimiters",
			content: "no frontmatter here",
			wantErr: true,
			errCode: clierrors.VALIDATION_ERROR,
		},
		{
			name: "missing closing delimiter",
			content: `---
key: PROJ-1
summary: Test
`,
			wantErr: true,
			errCode: clierrors.VALIDATION_ERROR,
		},
		{
			name: "invalid YAML",
			content: `---
key: [invalid yaml
---
`,
			wantErr: true,
			errCode: clierrors.VALIDATION_ERROR,
		},
		{
			name: "missing key field",
			content: `---
summary: No key here
---
Body text
`,
			wantErr: true,
			errCode: clierrors.VALIDATION_ERROR,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeTestFile(t, dir, "test.md", tt.content)

			got, err := ParseFile(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var cliErr *clierrors.CLIError
				if !errors.As(err, &cliErr) {
					t.Fatalf("expected CLIError, got %T: %v", err, err)
				}
				if cliErr.Code != tt.errCode {
					t.Errorf("error code = %q, want %q", cliErr.Code, tt.errCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFile() error = %v", err)
			}

			if got.Frontmatter.Key != tt.wantKey {
				t.Errorf("key = %q, want %q", got.Frontmatter.Key, tt.wantKey)
			}
			if got.Description != tt.wantBody {
				t.Errorf("description = %q, want %q", got.Description, tt.wantBody)
			}
			if got.Path != path {
				t.Errorf("path = %q, want %q", got.Path, path)
			}
		})
	}
}

func TestParseFileFrontmatterFields(t *testing.T) {
	dir := t.TempDir()
	path := writeTestFile(t, dir, "test.md", validFile)

	got, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	fm := got.Frontmatter
	checks := []struct {
		field string
		got   string
		want  string
	}{
		{"key", fm.Key, "PROJ-123"},
		{"summary", fm.Summary, "Fix login bug"},
		{"type", fm.Type, "Bug"},
		{"status", fm.Status, "In Progress"},
		{"priority", fm.Priority, "High"},
		{"project", fm.Project, "PROJ"},
		{"assignee", fm.Assignee, "Alice"},
		{"assignee_id", fm.AssigneeID, "abc123"},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("frontmatter.%s = %q, want %q", c.field, c.got, c.want)
		}
	}

	if len(fm.Labels) != 2 || fm.Labels[0] != "backend" || fm.Labels[1] != "urgent" {
		t.Errorf("labels = %v, want [backend urgent]", fm.Labels)
	}
}

func TestParseFileNotFound(t *testing.T) {
	_, err := ParseFile("/nonexistent/file.md")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
}

func TestIsCreate(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"PROJ-NEW-1", true},
		{"PROJ-NEW-42", true},
		{"ABC-NEW-999", true},
		{"PROJ-123", false},
		{"proj-NEW-1", false}, // lowercase project
		{"PROJ-NEW-", false},  // no number
		{"PROJ-NEW-0", true},
		{"PROJ-OLD-1", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			f := &IssueFile{Frontmatter: Frontmatter{Key: tt.key}}
			if got := f.IsCreate(); got != tt.want {
				t.Errorf("IsCreate() for key %q = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestParseDir(t *testing.T) {
	dir := t.TempDir()

	// Create files in nested structure
	writeTestFile(t, dir, "PROJ/PROJ-001 - First.md", `---
key: PROJ-001
summary: First issue
---
First body
`)
	writeTestFile(t, dir, "PROJ/PROJ-002 - Second.md", `---
key: PROJ-002
summary: Second issue
---
Second body
`)
	writeTestFile(t, dir, "OTHER/OTHER-001 - Third.md", `---
key: OTHER-001
summary: Third issue
---
Third body
`)

	// Non-md file should be skipped
	writeTestFile(t, dir, "PROJ/README.txt", "not a markdown file")

	files, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir() error = %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	// Should be sorted by path
	wantKeys := []string{"OTHER-001", "PROJ-001", "PROJ-002"}
	for i, wk := range wantKeys {
		if files[i].Frontmatter.Key != wk {
			t.Errorf("file[%d].Key = %q, want %q", i, files[i].Frontmatter.Key, wk)
		}
	}
}

func TestParseDirEmpty(t *testing.T) {
	dir := t.TempDir()

	files, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir() error = %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestParseDirInvalidFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "bad.md", "no frontmatter")

	_, err := ParseDir(dir)
	if err == nil {
		t.Fatal("expected error for invalid file in dir")
	}
	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
}
