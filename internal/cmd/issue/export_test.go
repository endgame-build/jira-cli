package issue

import (
	"encoding/json"
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
// responses, optionally capturing the JQL query.
func exportSearchHandler(t *testing.T, pages [][]api.Issue, capturedJQL *string) http.HandlerFunc {
	t.Helper()
	callCount := 0
	return func(w http.ResponseWriter, r *http.Request) {
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
