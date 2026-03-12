package issue

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/config"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/markdown"

	"errors"
)

// exportIssues returns two sample issues with all fields populated for export tests.
func exportIssues() []api.Issue {
	return []api.Issue{
		{
			ID:  "10001",
			Key: "PROJ-1",
			Fields: api.IssueFields{
				Summary:     "First issue",
				Description: json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"Hello world"}]}]}`),
				Status:      &api.Status{ID: "1", Name: "To Do"},
				IssueType:   &api.IssueType{ID: "10001", Name: "Bug"},
				Priority:    &api.Priority{ID: "2", Name: "High"},
				Labels:      []string{"backend"},
				Project:     &api.Project{ID: "1001", Key: "PROJ", Name: "Project"},
				Assignee:    &api.User{AccountID: "abc123", DisplayName: "Jane Doe"},
				Reporter:    &api.User{AccountID: "def456", DisplayName: "John Smith"},
				Created:     "2026-01-01T00:00:00.000+0000",
				Updated:     "2026-01-02T00:00:00.000+0000",
			},
		},
		{
			ID:  "10002",
			Key: "PROJ-2",
			Fields: api.IssueFields{
				Summary:   "Second issue",
				Status:    &api.Status{ID: "3", Name: "In Progress"},
				IssueType: &api.IssueType{ID: "10002", Name: "Story"},
				Priority:  &api.Priority{ID: "3", Name: "Medium"},
				Project:   &api.Project{ID: "1001", Key: "PROJ", Name: "Project"},
				Created:   "2026-02-01T00:00:00.000+0000",
				Updated:   "2026-02-02T00:00:00.000+0000",
			},
		},
	}
}

// exportSearchHandler returns an HTTP handler that serves POST /search/jql
// and GET /field responses, optionally capturing the JQL query.
func exportSearchHandler(t *testing.T, pages [][]api.Issue, capturedJQL *string) http.HandlerFunc {
	t.Helper()
	callCount := 0
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/field") {
			json.NewEncoder(w).Encode([]api.Field{})
			return
		}

		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jql") {
			var req searchRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode search request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if capturedJQL != nil {
				*capturedJQL = req.JQL
			}

			page := callCount
			if page >= len(pages) {
				page = len(pages) - 1
			}
			callCount++

			isLast := callCount >= len(pages)
			nextToken := ""
			if !isLast {
				nextToken = "page-token-next"
			}

			resp := api.SearchResults{
				Issues:        pages[page],
				IsLast:        isLast,
				NextPageToken: nextToken,
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func TestExportBasic(t *testing.T) {
	issues := exportIssues()
	var capturedJQL string
	f, tio, _ := newTestCreateFactory(t,
		exportSearchHandler(t, [][]api.Issue{issues}, &capturedJQL), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify JQL.
	if !strings.Contains(capturedJQL, "PROJ") {
		t.Errorf("JQL should contain project, got: %s", capturedJQL)
	}

	// Verify files written.
	for _, issue := range issues {
		relPath := markdown.IssuePath(issue)
		fullPath := filepath.Join(outDir, relPath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", relPath)
		}
	}

	// Verify text output.
	out := tio.OutBuf.String()
	if !strings.Contains(out, "Exported 2 issues") {
		t.Errorf("expected 'Exported 2 issues' in output, got: %s", out)
	}
}

func TestExportFileContent(t *testing.T) {
	issues := exportIssues()
	f, _, _ := newTestCreateFactory(t,
		exportSearchHandler(t, [][]api.Issue{issues}, nil), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read first issue's file and verify content.
	relPath := markdown.IssuePath(issues[0])
	data, err := os.ReadFile(filepath.Join(outDir, relPath))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	content := string(data)

	// Verify frontmatter fields.
	for _, want := range []string{"key: PROJ-1", "summary: First issue", "status: To Do", "type: Bug", "priority: High"} {
		if !strings.Contains(content, want) {
			t.Errorf("file content missing %q:\n%s", want, content)
		}
	}

	// Verify markdown body from ADF description.
	if !strings.Contains(content, "Hello world") {
		t.Errorf("file content missing description body:\n%s", content)
	}
}

func TestExportDryRun(t *testing.T) {
	issues := exportIssues()
	f, tio, _ := newTestCreateFactory(t,
		exportSearchHandler(t, [][]api.Issue{issues}, nil), nil)
	f.DryRun = true

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify no files written to disk.
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 0 {
		t.Errorf("dry-run should not write files, found %d entries", len(entries))
	}

	// Verify output mentions file paths.
	out := tio.OutBuf.String()
	if !strings.Contains(out, "DRY RUN") {
		t.Errorf("expected 'DRY RUN' in output, got: %s", out)
	}
	if !strings.Contains(out, "Would export 2 issues") {
		t.Errorf("expected 'Would export 2 issues' in output, got: %s", out)
	}
}

func TestExportDryRunJSON(t *testing.T) {
	issues := exportIssues()
	f, tio, _ := newTestCreateFactory(t,
		exportSearchHandler(t, [][]api.Issue{issues}, nil), nil)
	f.DryRun = true
	f.OutputJSON = true

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if result["dry_run"] != true {
		t.Errorf("dry_run = %v, want true", result["dry_run"])
	}
}

func TestExportLimit(t *testing.T) {
	issues := exportIssues() // 2 issues
	f, _, _ := newTestCreateFactory(t,
		exportSearchHandler(t, [][]api.Issue{issues}, nil), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
		Limit:     1,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only 1 file should be written.
	var fileCount int
	filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			fileCount++
		}
		return nil
	})
	if fileCount != 1 {
		t.Errorf("expected 1 file with --limit=1, got %d", fileCount)
	}
}

func TestExportJSON(t *testing.T) {
	issues := exportIssues()
	f, tio, _ := newTestCreateFactory(t,
		exportSearchHandler(t, [][]api.Issue{issues}, nil), nil)
	f.OutputJSON = true

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if result["ok"] != true {
		t.Errorf("ok = %v, want true", result["ok"])
	}
	exported, ok := result["exported"].(float64)
	if !ok || exported != 2 {
		t.Errorf("exported = %v, want 2", result["exported"])
	}
	files, ok := result["files"].([]interface{})
	if !ok || len(files) != 2 {
		t.Errorf("files should have 2 entries, got %v", result["files"])
	}
	if result["output_dir"] == nil {
		t.Error("output_dir should be present")
	}
}

func TestExportJQL(t *testing.T) {
	issues := exportIssues()
	var capturedJQL string
	f, _, _ := newTestCreateFactory(t,
		exportSearchHandler(t, [][]api.Issue{issues}, &capturedJQL), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		JQL:       "project = PROJ AND status = Done",
		OutputDir: outDir,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// JQL should be passed directly without project resolution.
	if capturedJQL != "project = PROJ AND status = Done" {
		t.Errorf("JQL should be passed directly, got: %s", capturedJQL)
	}
}

func TestExportNoProject(t *testing.T) {
	f, _, _ := newTestCreateFactory(t,
		exportSearchHandler(t, [][]api.Issue{}, nil), nil)

	opts := &ExportOptions{
		Factory:   f,
		OutputDir: t.TempDir(),
	}

	err := runExport(opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.VALIDATION_ERROR)
	}
	if !strings.Contains(cliErr.Message, "--project") {
		t.Errorf("error message should mention --project: %s", cliErr.Message)
	}
}

func TestExportNoProjectWithConfig(t *testing.T) {
	issues := exportIssues()
	var capturedJQL string
	f, _, _ := newTestCreateFactory(t,
		exportSearchHandler(t, [][]api.Issue{issues}, &capturedJQL),
		func(cfg config.Config) {
			if err := cfg.Set("default.project", "CONF"); err != nil {
				panic(err)
			}
			cfg.Save()
		})

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		OutputDir: outDir,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(capturedJQL, "CONF") {
		t.Errorf("JQL should use config project CONF, got: %s", capturedJQL)
	}
}

func TestExportPagination(t *testing.T) {
	page1 := []api.Issue{exportIssues()[0]}
	page2 := []api.Issue{exportIssues()[1]}

	f, _, _ := newTestCreateFactory(t,
		exportSearchHandler(t, [][]api.Issue{page1, page2}, nil), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Both pages' issues should be written.
	var fileCount int
	filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			fileCount++
		}
		return nil
	})
	if fileCount != 2 {
		t.Errorf("expected 2 files from 2 pages, got %d", fileCount)
	}
}

func TestExportEmpty(t *testing.T) {
	f, tio, _ := newTestCreateFactory(t,
		exportSearchHandler(t, [][]api.Issue{{}}, nil), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No files should be written.
	entries, _ := os.ReadDir(outDir)
	if len(entries) != 0 {
		t.Errorf("expected 0 files for empty results, found %d", len(entries))
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Exported 0 issues") {
		t.Errorf("expected 'Exported 0 issues' in output, got: %s", out)
	}
}

// treeExportIssues returns issues for tree-mode tests:
// 1 Epic, 2 Stories (parent = epic), 1 orphan Bug.
func treeExportIssues() []api.Issue {
	return []api.Issue{
		{
			ID:  "10001",
			Key: "PROJ-1",
			Fields: api.IssueFields{
				Summary:   "Epic One",
				IssueType: &api.IssueType{ID: "10000", Name: "Epic"},
				Project:   &api.Project{ID: "1001", Key: "PROJ", Name: "Project"},
				Status:    &api.Status{ID: "1", Name: "To Do"},
				Created:   "2026-01-01T00:00:00.000+0000",
				Updated:   "2026-01-02T00:00:00.000+0000",
			},
		},
		{
			ID:  "10010",
			Key: "PROJ-10",
			Fields: api.IssueFields{
				Summary:   "Story One",
				IssueType: &api.IssueType{ID: "10001", Name: "Story"},
				Project:   &api.Project{ID: "1001", Key: "PROJ", Name: "Project"},
				Status:    &api.Status{ID: "1", Name: "To Do"},
				Parent: &api.IssueParent{
					Key:    "PROJ-1",
					Fields: &api.ParentFields{Summary: "Epic One"},
				},
				Created: "2026-01-01T00:00:00.000+0000",
				Updated: "2026-01-02T00:00:00.000+0000",
			},
		},
		{
			ID:  "10011",
			Key: "PROJ-11",
			Fields: api.IssueFields{
				Summary:   "Story Two",
				IssueType: &api.IssueType{ID: "10001", Name: "Story"},
				Project:   &api.Project{ID: "1001", Key: "PROJ", Name: "Project"},
				Status:    &api.Status{ID: "1", Name: "To Do"},
				Parent: &api.IssueParent{
					Key:    "PROJ-1",
					Fields: &api.ParentFields{Summary: "Epic One"},
				},
				Created: "2026-01-01T00:00:00.000+0000",
				Updated: "2026-01-02T00:00:00.000+0000",
			},
		},
		{
			ID:  "10050",
			Key: "PROJ-50",
			Fields: api.IssueFields{
				Summary:   "Orphan Bug",
				IssueType: &api.IssueType{ID: "10002", Name: "Bug"},
				Project:   &api.Project{ID: "1001", Key: "PROJ", Name: "Project"},
				Status:    &api.Status{ID: "1", Name: "To Do"},
				Created:   "2026-01-01T00:00:00.000+0000",
				Updated:   "2026-01-02T00:00:00.000+0000",
			},
		},
	}
}

func TestExportTree(t *testing.T) {
	issues := treeExportIssues()
	f, _, _ := newTestCreateFactory(t,
		exportSearchHandler(t, [][]api.Issue{issues}, nil), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
		Tree:      true,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify hierarchical file paths.
	wantPaths := []string{
		"PROJ/PROJ-1 - Epic One/_epic.md",
		"PROJ/PROJ-1 - Epic One/PROJ-10 - Story One.md",
		"PROJ/PROJ-1 - Epic One/PROJ-11 - Story Two.md",
		"PROJ/PROJ-50 - Orphan Bug.md",
	}
	for _, rel := range wantPaths {
		full := filepath.Join(outDir, rel)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", rel)
		}
	}
}

func TestExportTreeDefault(t *testing.T) {
	issues := treeExportIssues()
	f, _, _ := newTestCreateFactory(t,
		exportSearchHandler(t, [][]api.Issue{issues}, nil), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
		// Tree not set — should produce flat layout.
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify flat file paths (no epic directories).
	for _, issue := range issues {
		relPath := markdown.IssuePath(issue)
		fullPath := filepath.Join(outDir, relPath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Errorf("expected flat file %s to exist", relPath)
		}
	}

	// Verify no _epic.md exists.
	found := false
	filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.Name() == "_epic.md" {
			found = true
		}
		return nil
	})
	if found {
		t.Error("flat export should not produce _epic.md files")
	}
}

func TestExportProgressStderr(t *testing.T) {
	// Create 100 issues to trigger progress reporting (every 50).
	issues := make([]api.Issue, 100)
	for i := range issues {
		issues[i] = api.Issue{
			ID:  string(rune('0' + i)),
			Key: "PROJ-" + strconv.Itoa(i+1),
			Fields: api.IssueFields{
				Summary: "Issue " + strconv.Itoa(i+1),
				Project: &api.Project{ID: "1001", Key: "PROJ", Name: "Project"},
			},
		}
	}

	f, tio, _ := newTestCreateFactory(t,
		exportSearchHandler(t, [][]api.Issue{issues}, nil), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	errOut := tio.ErrBuf.String()
	if !strings.Contains(errOut, "Exported 50 issues") {
		t.Errorf("expected progress at 50 issues in stderr, got: %s", errOut)
	}
	if !strings.Contains(errOut, "Exported 100 issues") {
		t.Errorf("expected progress at 100 issues in stderr, got: %s", errOut)
	}
}

// customFieldTestFields is shared field metadata for custom field tests (export + import).
var customFieldTestFields = []api.Field{
	{ID: "summary", Name: "Summary"},
	{ID: "status", Name: "Status"},
	{ID: "customfield_10001", Name: "Team", Custom: true, Schema: api.FieldSchema{
		Type:   "any",
		Custom: "com.atlassian.teams:rm-teams-custom-field-team",
	}},
	{ID: "customfield_10002", Name: "Story Points", Custom: true, Schema: api.FieldSchema{
		Type: "number",
	}},
}

// issueWithCustomFieldsJSON builds a raw JSON issue object that includes custom
// field keys at the fields level. This is necessary because IssueFields.CustomFields
// is json:"-" and won't survive standard JSON marshaling through the mock server.
func issueWithCustomFieldsJSON(issue api.Issue, customFields map[string]json.RawMessage) json.RawMessage {
	// Marshal the issue to get the base JSON.
	base, _ := json.Marshal(issue)
	var raw map[string]json.RawMessage
	json.Unmarshal(base, &raw)

	// Unmarshal the fields object and inject custom fields.
	var fields map[string]json.RawMessage
	json.Unmarshal(raw["fields"], &fields)
	for k, v := range customFields {
		fields[k] = v
	}
	fieldsJSON, _ := json.Marshal(fields)
	raw["fields"] = fieldsJSON

	result, _ := json.Marshal(raw)
	return result
}

// customFieldExportHandler returns an HTTP handler that serves GET /field with
// the given field definitions, and POST /search/jql with raw JSON issue responses
// that include custom field keys.
func customFieldExportHandler(t *testing.T, fields []api.Field, rawIssuePages [][]json.RawMessage) http.HandlerFunc {
	t.Helper()
	callCount := 0
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/field") {
			json.NewEncoder(w).Encode(fields)
			return
		}

		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jql") {
			var req searchRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode search request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			page := callCount
			if page >= len(rawIssuePages) {
				page = len(rawIssuePages) - 1
			}
			callCount++

			isLast := callCount >= len(rawIssuePages)
			nextToken := ""
			if !isLast {
				nextToken = "page-token-next"
			}

			// Build raw JSON response with issues array.
			issuesJSON, _ := json.Marshal(rawIssuePages[page])
			resp := fmt.Sprintf(`{"issues":%s,"isLast":%v,"nextPageToken":%q}`,
				issuesJSON, isLast, nextToken)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(resp))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func TestExportCustomFields(t *testing.T) {
	fields := customFieldTestFields
	baseIssue := exportIssues()[0]
	rawIssue := issueWithCustomFieldsJSON(baseIssue, map[string]json.RawMessage{
		"customfield_10001": json.RawMessage(`"Platform"`),
		"customfield_10002": json.RawMessage(`5`),
	})

	f, _, _ := newTestCreateFactory(t,
		customFieldExportHandler(t, fields, [][]json.RawMessage{{rawIssue}}), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	relPath := markdown.IssuePath(baseIssue)
	data, err := os.ReadFile(filepath.Join(outDir, relPath))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "team: Platform") {
		t.Errorf("expected 'team: Platform' in output:\n%s", content)
	}
	if !strings.Contains(content, "story_points: 5") {
		t.Errorf("expected 'story_points: 5' in output:\n%s", content)
	}
	if !strings.Contains(content, "key: PROJ-1") {
		t.Errorf("expected built-in 'key: PROJ-1' in output:\n%s", content)
	}
}

func TestExportFieldsFlag(t *testing.T) {
	fields := customFieldTestFields
	baseIssue := exportIssues()[0]
	rawIssue := issueWithCustomFieldsJSON(baseIssue, map[string]json.RawMessage{
		"customfield_10001": json.RawMessage(`"Platform"`),
		"customfield_10002": json.RawMessage(`5`),
	})

	f, _, _ := newTestCreateFactory(t,
		customFieldExportHandler(t, fields, [][]json.RawMessage{{rawIssue}}), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
		Fields:    "team",
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	relPath := markdown.IssuePath(baseIssue)
	data, err := os.ReadFile(filepath.Join(outDir, relPath))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "team: Platform") {
		t.Errorf("expected 'team: Platform' in output:\n%s", content)
	}
	if strings.Contains(content, "story_points") {
		t.Errorf("story_points should be filtered out:\n%s", content)
	}
}

func TestExportFieldsFlagAllUnknown(t *testing.T) {
	fields := customFieldTestFields
	baseIssue := exportIssues()[0]
	rawIssue := issueWithCustomFieldsJSON(baseIssue, nil)

	f, _, _ := newTestCreateFactory(t,
		customFieldExportHandler(t, fields, [][]json.RawMessage{{rawIssue}}), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
		Fields:    "nonexistent",
	}

	err := runExport(opts)
	if err == nil {
		t.Fatal("expected error when all --fields names are unmatched, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.VALIDATION_ERROR)
	}
}

func TestExportFieldsFlagPartialMatch(t *testing.T) {
	fields := customFieldTestFields
	baseIssue := exportIssues()[0]
	rawIssue := issueWithCustomFieldsJSON(baseIssue, map[string]json.RawMessage{
		"customfield_10001": json.RawMessage(`"Platform"`),
	})

	f, tio, _ := newTestCreateFactory(t,
		customFieldExportHandler(t, fields, [][]json.RawMessage{{rawIssue}}), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
		Fields:    "team,nonexistent",
	}

	// Should succeed (some matched), but warn about unmatched.
	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	errOut := tio.ErrBuf.String()
	if !strings.Contains(errOut, "nonexistent") {
		t.Errorf("expected warning about 'nonexistent' in stderr, got: %s", errOut)
	}
}

func TestExportBuiltinCollision(t *testing.T) {
	fields := []api.Field{
		{ID: "summary", Name: "Summary"},
		{ID: "status", Name: "Status"},
		{ID: "customfield_10099", Name: "Status", Custom: true},
		{ID: "customfield_10001", Name: "Team", Custom: true},
	}
	baseIssue := exportIssues()[0]
	rawIssue := issueWithCustomFieldsJSON(baseIssue, map[string]json.RawMessage{
		"customfield_10099": json.RawMessage(`"Blocked"`),
		"customfield_10001": json.RawMessage(`"Platform"`),
	})

	f, tio, _ := newTestCreateFactory(t,
		customFieldExportHandler(t, fields, [][]json.RawMessage{{rawIssue}}), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	relPath := markdown.IssuePath(baseIssue)
	data, err := os.ReadFile(filepath.Join(outDir, relPath))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "status: To Do") {
		t.Errorf("expected built-in 'status: To Do' in output:\n%s", content)
	}
	if strings.Contains(content, "Blocked") {
		t.Errorf("custom 'Status' field should be skipped, but found 'Blocked' in output:\n%s", content)
	}
	if !strings.Contains(content, "team: Platform") {
		t.Errorf("expected 'team: Platform' in output:\n%s", content)
	}

	errOut := tio.ErrBuf.String()
	if !strings.Contains(errOut, "status") {
		t.Errorf("expected warning about 'status' collision in stderr, got: %s", errOut)
	}
}

func TestExportCustomFieldObjectValue(t *testing.T) {
	fields := []api.Field{
		{ID: "customfield_10003", Name: "Severity", Custom: true},
	}
	baseIssue := exportIssues()[0]
	rawIssue := issueWithCustomFieldsJSON(baseIssue, map[string]json.RawMessage{
		"customfield_10003": json.RawMessage(`{"value": "Critical"}`),
	})

	f, _, _ := newTestCreateFactory(t,
		customFieldExportHandler(t, fields, [][]json.RawMessage{{rawIssue}}), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	relPath := markdown.IssuePath(baseIssue)
	data, err := os.ReadFile(filepath.Join(outDir, relPath))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "severity: Critical") {
		t.Errorf("expected 'severity: Critical' in output:\n%s", content)
	}
}

func TestExportCustomFieldSkipArray(t *testing.T) {
	fields := []api.Field{
		{ID: "customfield_10004", Name: "Components", Custom: true},
		{ID: "customfield_10001", Name: "Team", Custom: true},
	}
	baseIssue := exportIssues()[0]
	rawIssue := issueWithCustomFieldsJSON(baseIssue, map[string]json.RawMessage{
		"customfield_10004": json.RawMessage(`[{"name": "Backend"}, {"name": "Frontend"}]`),
		"customfield_10001": json.RawMessage(`"Platform"`),
	})

	f, tio, _ := newTestCreateFactory(t,
		customFieldExportHandler(t, fields, [][]json.RawMessage{{rawIssue}}), nil)

	outDir := t.TempDir()
	opts := &ExportOptions{
		Factory:   f,
		Project:   "PROJ",
		OutputDir: outDir,
	}

	if err := runExport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	relPath := markdown.IssuePath(baseIssue)
	data, err := os.ReadFile(filepath.Join(outDir, relPath))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	content := string(data)

	if strings.Contains(content, "components") {
		t.Errorf("array custom field 'components' should be skipped:\n%s", content)
	}
	if !strings.Contains(content, "team: Platform") {
		t.Errorf("expected 'team: Platform' in output:\n%s", content)
	}

	errOut := tio.ErrBuf.String()
	if !strings.Contains(errOut, "customfield_10004") || !strings.Contains(errOut, "skip") {
		t.Errorf("expected warning about skipped array field in stderr, got: %s", errOut)
	}
}
