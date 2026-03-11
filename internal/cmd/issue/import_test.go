package issue

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/auth"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/iostreams"
)

// writeImportFile writes a markdown file with frontmatter to the given directory.
func writeImportFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return path
}

// newTestImportFactory creates a factory wired to a test server.
func newTestImportFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams, *httptest.Server) {
	t.Helper()
	tio := iostreams.Test()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	creds := &auth.Credentials{
		Instance: "test.atlassian.net",
		User:     "test@example.com",
		Token:    "test-token",
	}
	client := api.NewClient(creds, api.WithBaseURL(srv.URL))
	f := factory.NewTestFactory(tio.IOStreams, nil, client)
	return f, tio, srv
}

// importHandler handles create (POST /issue), update (PUT /issue/{key}), get (GET /issue/{key}), and field metadata (GET /field).
func importHandler(t *testing.T, captureCreateBody *string, getIssueUpdated string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		// GET /field — field metadata for custom field resolution.
		if r.Method == http.MethodGet && r.URL.Path == "/field" {
			json.NewEncoder(w).Encode([]api.Field{})
			return
		}

		// POST /issue — create
		if r.Method == http.MethodPost && r.URL.Path == "/issue" {
			if captureCreateBody != nil {
				body, _ := io.ReadAll(r.Body)
				*captureCreateBody = string(body)
			}
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(api.CreatedIssue{
				ID:   "10042",
				Key:  "PROJ-124",
				Self: "https://test.atlassian.net/rest/api/3/issue/10042",
			})
			return
		}

		// GET /issue/{key} — get issue for conflict check
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/issue/") {
			key := strings.TrimPrefix(r.URL.Path, "/issue/")
			json.NewEncoder(w).Encode(api.Issue{
				ID:  "10001",
				Key: key,
				Fields: api.IssueFields{
					Summary: "Existing issue",
					Updated: getIssueUpdated,
				},
			})
			return
		}

		// PUT /issue/{key} — edit
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/issue/") {
			w.WriteHeader(204)
			return
		}

		w.WriteHeader(404)
		w.Write([]byte(`{"errorMessages":["Not found"]}`))
	}
}

func TestImportCreate(t *testing.T) {
	var capturedBody string
	f, tio, _ := newTestImportFactory(t, importHandler(t, &capturedBody, ""))

	dir := t.TempDir()
	path := writeImportFile(t, dir, "PROJ-NEW-1 - Test Issue.md", `---
key: PROJ-NEW-1
summary: Test Issue
type: Bug
project: PROJ
priority: High
labels:
  - backend
  - urgent
---
Some **bold** description
`)

	opts := &ImportOptions{
		Factory: f,
		Files:   []string{path},
	}

	if err := runImport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the create payload.
	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})

	// Project.
	proj := fields["project"].(map[string]interface{})
	if proj["key"] != "PROJ" {
		t.Errorf("project key = %v, want PROJ", proj["key"])
	}

	// Issue type.
	issueType := fields["issuetype"].(map[string]interface{})
	if issueType["name"] != "Bug" {
		t.Errorf("issuetype name = %v, want Bug", issueType["name"])
	}

	// Summary.
	if fields["summary"] != "Test Issue" {
		t.Errorf("summary = %v, want 'Test Issue'", fields["summary"])
	}

	// Priority.
	pri := fields["priority"].(map[string]interface{})
	if pri["name"] != "High" {
		t.Errorf("priority name = %v, want High", pri["name"])
	}

	// Labels.
	labels := fields["labels"].([]interface{})
	if len(labels) != 2 || labels[0] != "backend" || labels[1] != "urgent" {
		t.Errorf("labels = %v, want [backend urgent]", labels)
	}

	// Description should be ADF (*adf.Node set directly, not json.RawMessage).
	desc, ok := fields["description"].(map[string]interface{})
	if !ok {
		t.Fatalf("description should be ADF object, got %T", fields["description"])
	}
	if desc["type"] != "doc" {
		t.Errorf("description type = %v, want 'doc'", desc["type"])
	}

	// Verify output contains new key.
	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ-124") {
		t.Errorf("output should contain real key PROJ-124, got: %s", out)
	}
	if !strings.Contains(out, "PROJ-NEW-1") {
		t.Errorf("output should contain temp key PROJ-NEW-1, got: %s", out)
	}
}

func TestImportCreateWithAssignee(t *testing.T) {
	var capturedBody string
	f, _, _ := newTestImportFactory(t, importHandler(t, &capturedBody, ""))

	dir := t.TempDir()
	path := writeImportFile(t, dir, "create.md", `---
key: PROJ-NEW-1
summary: With Assignee
type: Task
project: PROJ
assignee_id: abc123def456
---
`)

	opts := &ImportOptions{
		Factory: f,
		Files:   []string{path},
	}

	if err := runImport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})

	assignee := fields["assignee"].(map[string]interface{})
	if assignee["accountId"] != "abc123def456" {
		t.Errorf("assignee accountId = %v, want abc123def456", assignee["accountId"])
	}
}

func TestImportCreateWithoutAssignee(t *testing.T) {
	var capturedBody string
	f, _, _ := newTestImportFactory(t, importHandler(t, &capturedBody, ""))

	dir := t.TempDir()
	path := writeImportFile(t, dir, "create.md", `---
key: PROJ-NEW-1
summary: No Assignee
type: Task
project: PROJ
---
`)

	opts := &ImportOptions{
		Factory: f,
		Files:   []string{path},
	}

	if err := runImport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})

	if _, ok := fields["assignee"]; ok {
		t.Error("assignee field should be omitted when assignee_id is empty")
	}
}

func TestImportTempToTempParent(t *testing.T) {
	f, _, _ := newTestImportFactory(t, importHandler(t, nil, ""))

	dir := t.TempDir()
	path := writeImportFile(t, dir, "child.md", `---
key: PROJ-NEW-2
summary: Child Issue
type: Task
project: PROJ
parent: PROJ-NEW-1
---
`)

	opts := &ImportOptions{
		Factory: f,
		Files:   []string{path},
	}

	err := runImport(opts)
	if err == nil {
		t.Fatal("expected error for temp-to-temp parent, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.VALIDATION_ERROR)
	}
	if !strings.Contains(cliErr.Message, "temp-to-temp") {
		t.Errorf("error message should mention temp-to-temp: %s", cliErr.Message)
	}
}

func TestImportCreateMissingProject(t *testing.T) {
	f, _, _ := newTestImportFactory(t, importHandler(t, nil, ""))

	dir := t.TempDir()
	path := writeImportFile(t, dir, "create.md", `---
key: PROJ-NEW-1
summary: Missing Project
type: Task
---
`)

	opts := &ImportOptions{
		Factory: f,
		Files:   []string{path},
	}

	err := runImport(opts)
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
	if !strings.Contains(cliErr.Message, "project") {
		t.Errorf("error message should mention project: %s", cliErr.Message)
	}
}

func TestImportCreateMissingType(t *testing.T) {
	f, _, _ := newTestImportFactory(t, importHandler(t, nil, ""))

	dir := t.TempDir()
	path := writeImportFile(t, dir, "create.md", `---
key: PROJ-NEW-1
summary: Missing Type
project: PROJ
---
`)

	opts := &ImportOptions{
		Factory: f,
		Files:   []string{path},
	}

	err := runImport(opts)
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
	if !strings.Contains(cliErr.Message, "type") {
		t.Errorf("error message should mention type: %s", cliErr.Message)
	}
}

func TestImportNoArgsNoDir(t *testing.T) {
	f, _, _ := newTestImportFactory(t, importHandler(t, nil, ""))

	cmd := NewCmdImport(f)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
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
}

func TestImportConflictMismatch(t *testing.T) {
	// Server returns updated="2026-01-02", file has updated="2026-01-01" — mismatch.
	f, _, _ := newTestImportFactory(t, importHandler(t, nil, "2026-01-02T00:00:00.000+0000"))

	dir := t.TempDir()
	path := writeImportFile(t, dir, "update.md", `---
key: PROJ-123
summary: Updated Title
updated: "2026-01-01T00:00:00.000+0000"
---
`)

	opts := &ImportOptions{
		Factory: f,
		Files:   []string{path},
	}

	err := runImport(opts)
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.CONFLICT_ERROR {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.CONFLICT_ERROR)
	}
}

func TestImportConflictMatching(t *testing.T) {
	// Matching timestamps — should proceed without error.
	f, _, _ := newTestImportFactory(t, importHandler(t, nil, "2026-01-01T00:00:00.000+0000"))

	dir := t.TempDir()
	path := writeImportFile(t, dir, "update.md", `---
key: PROJ-123
summary: Updated Title
updated: "2026-01-01T00:00:00.000+0000"
---
`)

	opts := &ImportOptions{
		Factory: f,
		Files:   []string{path},
	}

	if err := runImport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImportConflictEmptyTimestamp(t *testing.T) {
	// Empty updated in frontmatter — skip conflict check.
	f, _, _ := newTestImportFactory(t, importHandler(t, nil, "2026-01-02T00:00:00.000+0000"))

	dir := t.TempDir()
	path := writeImportFile(t, dir, "update.md", `---
key: PROJ-123
summary: Updated Title
---
`)

	opts := &ImportOptions{
		Factory: f,
		Files:   []string{path},
	}

	if err := runImport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImportForceOverride(t *testing.T) {
	// Mismatched timestamps but --force is set — should succeed.
	f, _, _ := newTestImportFactory(t, importHandler(t, nil, "2026-01-02T00:00:00.000+0000"))

	dir := t.TempDir()
	path := writeImportFile(t, dir, "update.md", `---
key: PROJ-123
summary: Force Updated
updated: "2026-01-01T00:00:00.000+0000"
---
`)

	opts := &ImportOptions{
		Factory: f,
		Files:   []string{path},
		Force:   true,
	}

	if err := runImport(opts); err != nil {
		t.Fatalf("unexpected error with --force: %v", err)
	}
}

func TestImportDryRun(t *testing.T) {
	apiCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/field" {
			json.NewEncoder(w).Encode([]api.Field{})
			return
		}
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			apiCalled = true
		}
		w.WriteHeader(404)
	})

	f, tio, _ := newTestImportFactory(t, handler)
	f.DryRun = true

	dir := t.TempDir()
	writeImportFile(t, dir, "PROJ/PROJ-NEW-1 - New.md", `---
key: PROJ-NEW-1
summary: New Issue
type: Task
project: PROJ
---
`)
	writeImportFile(t, dir, "PROJ/PROJ-123 - Existing.md", `---
key: PROJ-123
summary: Existing Issue
---
`)

	opts := &ImportOptions{
		Factory: f,
		Dir:     dir,
	}

	if err := runImport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if apiCalled {
		t.Error("dry-run should not make API calls")
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "DRY RUN") {
		t.Errorf("output should contain 'DRY RUN', got: %s", out)
	}
}

func TestImportStopOnFirstError(t *testing.T) {
	callCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/field" {
			json.NewEncoder(w).Encode([]api.Field{})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/issue" {
			callCount++
			// First create fails.
			w.WriteHeader(400)
			w.Write([]byte(`{"errorMessages":["Bad request"]}`))
			return
		}
		w.WriteHeader(404)
	})

	f, _, _ := newTestImportFactory(t, handler)

	dir := t.TempDir()
	path1 := writeImportFile(t, dir, "first.md", `---
key: PROJ-NEW-1
summary: First
type: Task
project: PROJ
---
`)
	path2 := writeImportFile(t, dir, "second.md", `---
key: PROJ-NEW-2
summary: Second
type: Task
project: PROJ
---
`)

	opts := &ImportOptions{
		Factory: f,
		Files:   []string{path1, path2},
	}

	err := runImport(opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if callCount != 1 {
		t.Errorf("expected 1 API call (stop on first error), got %d", callCount)
	}
}

func TestImportMixed(t *testing.T) {
	createCalls := 0
	editCalls := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/field" {
			json.NewEncoder(w).Encode([]api.Field{})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/issue" {
			createCalls++
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(api.CreatedIssue{
				ID:  "10042",
				Key: "PROJ-124",
			})
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/issue/") {
			key := strings.TrimPrefix(r.URL.Path, "/issue/")
			json.NewEncoder(w).Encode(api.Issue{
				ID:  "10001",
				Key: key,
				Fields: api.IssueFields{
					Summary: "Existing",
					Updated: "2026-01-01T00:00:00.000+0000",
				},
			})
			return
		}
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/issue/") {
			editCalls++
			w.WriteHeader(204)
			return
		}
		w.WriteHeader(404)
	})

	f, tio, _ := newTestImportFactory(t, handler)
	f.OutputJSON = true

	dir := t.TempDir()
	path1 := writeImportFile(t, dir, "create.md", `---
key: PROJ-NEW-1
summary: New Issue
type: Bug
project: PROJ
---
`)
	path2 := writeImportFile(t, dir, "update.md", `---
key: PROJ-100
summary: Updated Issue
updated: "2026-01-01T00:00:00.000+0000"
---
`)

	opts := &ImportOptions{
		Factory: f,
		Files:   []string{path1, path2},
	}

	if err := runImport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if createCalls != 1 {
		t.Errorf("expected 1 create call, got %d", createCalls)
	}
	if editCalls != 1 {
		t.Errorf("expected 1 edit call, got %d", editCalls)
	}

	// Verify JSON output.
	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if result["ok"] != true {
		t.Errorf("ok = %v, want true", result["ok"])
	}
	created := result["created"].(float64)
	updated := result["updated"].(float64)
	if created != 1 {
		t.Errorf("created = %v, want 1", created)
	}
	if updated != 1 {
		t.Errorf("updated = %v, want 1", updated)
	}

	results := result["results"].([]interface{})
	if len(results) != 2 {
		t.Errorf("results length = %d, want 2", len(results))
	}

	// First result should be the create.
	r0 := results[0].(map[string]interface{})
	if r0["action"] != "created" {
		t.Errorf("result[0].action = %v, want 'created'", r0["action"])
	}
	if r0["key"] != "PROJ-124" {
		t.Errorf("result[0].key = %v, want PROJ-124", r0["key"])
	}
	if r0["temp_key"] != "PROJ-NEW-1" {
		t.Errorf("result[0].temp_key = %v, want PROJ-NEW-1", r0["temp_key"])
	}
	if r0["url"] == nil || r0["url"] == "" {
		t.Error("result[0].url should contain browse URL")
	}

	// Second result should be the update.
	r1 := results[1].(map[string]interface{})
	if r1["action"] != "updated" {
		t.Errorf("result[1].action = %v, want 'updated'", r1["action"])
	}
	if r1["key"] != "PROJ-100" {
		t.Errorf("result[1].key = %v, want PROJ-100", r1["key"])
	}
}

func TestImportUpdateFieldsExcluded(t *testing.T) {
	var capturedBody string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/field" {
			json.NewEncoder(w).Encode([]api.Field{})
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/issue/") {
			json.NewEncoder(w).Encode(api.Issue{
				ID:  "10001",
				Key: "PROJ-123",
				Fields: api.IssueFields{
					Summary: "Existing",
				},
			})
			return
		}
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/issue/") {
			body, _ := io.ReadAll(r.Body)
			capturedBody = string(body)
			w.WriteHeader(204)
			return
		}
		w.WriteHeader(404)
	})

	f, _, _ := newTestImportFactory(t, handler)

	dir := t.TempDir()
	path := writeImportFile(t, dir, "update.md", `---
key: PROJ-123
summary: Updated Summary
type: Bug
project: PROJ
status: Done
parent: PROJ-100
priority: High
---
Updated description
`)

	opts := &ImportOptions{
		Factory: f,
		Files:   []string{path},
	}

	if err := runImport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify update payload does NOT contain type, project, parent, or status.
	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})

	if _, ok := fields["issuetype"]; ok {
		t.Error("update should not send issuetype")
	}
	if _, ok := fields["project"]; ok {
		t.Error("update should not send project")
	}
	if _, ok := fields["parent"]; ok {
		t.Error("update should not send parent")
	}
	if _, ok := fields["status"]; ok {
		t.Error("update should not send status")
	}

	// But should send summary and priority.
	if fields["summary"] != "Updated Summary" {
		t.Errorf("summary = %v, want 'Updated Summary'", fields["summary"])
	}
	pri := fields["priority"].(map[string]interface{})
	if pri["name"] != "High" {
		t.Errorf("priority name = %v, want High", pri["name"])
	}

	// Description should be ADF.
	desc, ok := fields["description"].(map[string]interface{})
	if !ok {
		t.Fatalf("description should be ADF object, got %T", fields["description"])
	}
	if desc["type"] != "doc" {
		t.Errorf("description type = %v, want 'doc'", desc["type"])
	}
}

func TestImportCreateWithParent(t *testing.T) {
	var capturedBody string
	f, _, _ := newTestImportFactory(t, importHandler(t, &capturedBody, ""))

	dir := t.TempDir()
	path := writeImportFile(t, dir, "child.md", `---
key: PROJ-NEW-1
summary: Child Issue
type: Task
project: PROJ
parent: PROJ-100
---
`)

	opts := &ImportOptions{
		Factory: f,
		Files:   []string{path},
	}

	if err := runImport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})

	parent := fields["parent"].(map[string]interface{})
	if parent["key"] != "PROJ-100" {
		t.Errorf("parent key = %v, want PROJ-100", parent["key"])
	}
}

func TestImportDirFlag(t *testing.T) {
	f, _, _ := newTestImportFactory(t, importHandler(t, nil, ""))

	dir := t.TempDir()
	writeImportFile(t, dir, "PROJ/PROJ-NEW-1 - Issue.md", `---
key: PROJ-NEW-1
summary: From Dir
type: Task
project: PROJ
---
`)

	opts := &ImportOptions{
		Factory: f,
		Dir:     dir,
	}

	if err := runImport(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestImportFilesAndDirMutuallyExclusive(t *testing.T) {
	f, _, _ := newTestImportFactory(t, importHandler(t, nil, ""))

	cmd := NewCmdImport(f)
	cmd.SetArgs([]string{"file.md", "--dir", "/some/dir"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for files + --dir, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.VALIDATION_ERROR)
	}
}
