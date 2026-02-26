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

	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/auth"
	"github.com/endgameio/jira-cli/internal/config"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/iostreams"
)

// newTestCreateFactory creates a Factory wired to a test httptest server with config.
func newTestCreateFactory(t *testing.T, handler http.Handler, cfgSetup func(config.Config)) (*factory.Factory, *iostreams.TestIOStreams, *httptest.Server) {
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

	var cfg config.Config
	if cfgSetup != nil {
		cfgPath := t.TempDir() + "/config.toml"
		var err error
		cfg, err = config.LoadFromPath(cfgPath)
		if err != nil {
			t.Fatalf("load config: %v", err)
		}
		cfgSetup(cfg)
	}

	f := factory.NewTestFactory(tio.IOStreams, cfg, client)
	return f, tio, srv
}

// createHandler returns an HTTP handler that serves POST /issue responses
// and GET /issue/createmeta responses. It captures the request body for verification.
func createHandler(captureBody *string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/issue" {
			if captureBody != nil {
				body, _ := io.ReadAll(r.Body)
				*captureBody = string(body)
			}
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(api.CreatedIssue{
				ID:   "10042",
				Key:  "PROJ-124",
				Self: "https://test.atlassian.net/rest/api/3/issue/10042",
			})
			return
		}

		// Createmeta endpoint for dry-run validation.
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "createmeta") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"issueTypes": []map[string]interface{}{
					{"id": "10001", "name": "Bug", "subtask": false},
					{"id": "10002", "name": "Story", "subtask": false},
					{"id": "10003", "name": "Task", "subtask": false},
				},
			})
			return
		}

		// User search endpoint for assignee resolution.
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/user/search") {
			json.NewEncoder(w).Encode([]api.User{
				{
					AccountID:   "abc123def456abc123def456",
					DisplayName: "Jane Doe",
				},
			})
			return
		}

		// GET /myself for @me resolution.
		if r.Method == http.MethodGet && r.URL.Path == "/myself" {
			json.NewEncoder(w).Encode(api.User{
				AccountID:   "abc123def456abc123def456",
				DisplayName: "Test User",
			})
			return
		}

		w.WriteHeader(404)
		w.Write([]byte(`{"errorMessages":["Not found"]}`))
	}
}

func TestCreateSuccessful(t *testing.T) {
	var capturedBody string
	f, tio, _ := newTestCreateFactory(t, createHandler(&capturedBody), nil)

	opts := &CreateOptions{
		Factory: f,
		Project: "PROJ",
		Type:    "Bug",
		Summary: "Fix login bug",
	}

	if err := runCreate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ-124") {
		t.Errorf("output missing issue key: %s", out)
	}
	if !strings.Contains(out, "https://test.atlassian.net/browse/PROJ-124") {
		t.Errorf("output missing browse URL: %s", out)
	}

	// Verify request body contains correct fields.
	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})
	proj := fields["project"].(map[string]interface{})
	if proj["key"] != "PROJ" {
		t.Errorf("project key = %v, want PROJ", proj["key"])
	}
	issueType := fields["issuetype"].(map[string]interface{})
	if issueType["name"] != "Bug" {
		t.Errorf("issuetype name = %v, want Bug", issueType["name"])
	}
	if fields["summary"] != "Fix login bug" {
		t.Errorf("summary = %v, want 'Fix login bug'", fields["summary"])
	}
}

func TestCreateWithCustomFields(t *testing.T) {
	var capturedBody string
	f, _, _ := newTestCreateFactory(t, createHandler(&capturedBody), nil)

	opts := &CreateOptions{
		Factory:     f,
		Project:     "PROJ",
		Type:        "Story",
		Summary:     "New feature",
		Description: "Some **bold** text",
		Priority:    "High",
		Labels:      []string{"frontend", "urgent"},
		Parent:      "PROJ-100",
		Fields:      []string{"customfield_10001=high", "customfield_10002=value with = sign"},
	}

	if err := runCreate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})

	// Priority.
	pri := fields["priority"].(map[string]interface{})
	if pri["name"] != "High" {
		t.Errorf("priority = %v, want High", pri["name"])
	}

	// Labels.
	labels := fields["labels"].([]interface{})
	if len(labels) != 2 || labels[0] != "frontend" || labels[1] != "urgent" {
		t.Errorf("labels = %v, want [frontend urgent]", labels)
	}

	// Parent.
	parent := fields["parent"].(map[string]interface{})
	if parent["key"] != "PROJ-100" {
		t.Errorf("parent key = %v, want PROJ-100", parent["key"])
	}

	// Custom fields.
	if fields["customfield_10001"] != "high" {
		t.Errorf("customfield_10001 = %v, want 'high'", fields["customfield_10001"])
	}
	if fields["customfield_10002"] != "value with = sign" {
		t.Errorf("customfield_10002 = %v, want 'value with = sign'", fields["customfield_10002"])
	}

	// Description should be ADF (has "type": "doc").
	desc, ok := fields["description"].(map[string]interface{})
	if !ok {
		t.Fatalf("description should be ADF object, got %T", fields["description"])
	}
	if desc["type"] != "doc" {
		t.Errorf("description type = %v, want 'doc'", desc["type"])
	}
}

func TestCreateMissingProject(t *testing.T) {
	f, _, _ := newTestCreateFactory(t, createHandler(nil), nil)

	opts := &CreateOptions{
		Factory: f,
		Type:    "Bug",
		Summary: "Fix login bug",
	}

	err := runCreate(opts)
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

func TestCreateMissingType(t *testing.T) {
	f, _, _ := newTestCreateFactory(t, createHandler(nil), nil)

	opts := &CreateOptions{
		Factory: f,
		Project: "PROJ",
		Summary: "Fix login bug",
	}

	err := runCreate(opts)
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

func TestCreateMissingSummary(t *testing.T) {
	f, _, _ := newTestCreateFactory(t, createHandler(nil), nil)

	opts := &CreateOptions{
		Factory: f,
		Project: "PROJ",
		Type:    "Bug",
	}

	err := runCreate(opts)
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

func TestCreateMissingAllRequired(t *testing.T) {
	f, _, _ := newTestCreateFactory(t, createHandler(nil), nil)

	opts := &CreateOptions{
		Factory: f,
	}

	err := runCreate(opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	// First error encountered is --project.
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.VALIDATION_ERROR)
	}
}

func TestCreateJSON(t *testing.T) {
	f, tio, _ := newTestCreateFactory(t, createHandler(nil), nil)
	f.OutputJSON = true

	opts := &CreateOptions{
		Factory: f,
		Project: "PROJ",
		Type:    "Bug",
		Summary: "Fix login bug",
	}

	if err := runCreate(opts); err != nil {
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
	if result["key"] != "PROJ-124" {
		t.Errorf("key = %v, want PROJ-124", result["key"])
	}
	if result["id"] != "10042" {
		t.Errorf("id = %v, want 10042", result["id"])
	}
	if result["url"] != "https://test.atlassian.net/browse/PROJ-124" {
		t.Errorf("url = %v, want browse URL", result["url"])
	}
}

func TestCreateQuiet(t *testing.T) {
	f, tio, _ := newTestCreateFactory(t, createHandler(nil), nil)
	f.Quiet = true

	opts := &CreateOptions{
		Factory: f,
		Project: "PROJ",
		Type:    "Bug",
		Summary: "Fix login bug",
	}

	if err := runCreate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if out != "" {
		t.Errorf("quiet mode should produce no output, got: %s", out)
	}
}

func TestCreateAPIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"errorMessages":["Field 'summary' is required"]}`))
	})

	f, _, _ := newTestCreateFactory(t, handler, nil)

	opts := &CreateOptions{
		Factory: f,
		Project: "PROJ",
		Type:    "Bug",
		Summary: "Test",
	}

	err := runCreate(opts)
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

func TestCreateFieldSplitOnFirstEquals(t *testing.T) {
	var capturedBody string
	f, _, _ := newTestCreateFactory(t, createHandler(&capturedBody), nil)

	opts := &CreateOptions{
		Factory: f,
		Project: "PROJ",
		Type:    "Bug",
		Summary: "Test",
		Fields:  []string{"cf_url=https://example.com?a=1&b=2"},
	}

	if err := runCreate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})

	want := "https://example.com?a=1&b=2"
	if fields["cf_url"] != want {
		t.Errorf("cf_url = %v, want %q", fields["cf_url"], want)
	}
}

func TestCreateFieldCollisionWarning(t *testing.T) {
	f, tio, _ := newTestCreateFactory(t, createHandler(nil), nil)

	opts := &CreateOptions{
		Factory: f,
		Project: "PROJ",
		Type:    "Bug",
		Summary: "Test",
		Fields:  []string{"summary=Ignored Value"},
	}

	if err := runCreate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	errOut := tio.ErrBuf.String()
	if !strings.Contains(errOut, "Warning") || !strings.Contains(errOut, "summary") {
		t.Errorf("stderr should contain warning about summary collision:\n%s", errOut)
	}
}

func TestCreateInvalidFieldFormat(t *testing.T) {
	f, _, _ := newTestCreateFactory(t, createHandler(nil), nil)

	opts := &CreateOptions{
		Factory: f,
		Project: "PROJ",
		Type:    "Bug",
		Summary: "Test",
		Fields:  []string{"no-equals-sign"},
	}

	err := runCreate(opts)
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

func TestCreateWithAssignee(t *testing.T) {
	var capturedBody string
	f, _, _ := newTestCreateFactory(t, createHandler(&capturedBody), nil)

	opts := &CreateOptions{
		Factory:  f,
		Project:  "PROJ",
		Type:     "Bug",
		Summary:  "Test",
		Assignee: "Jane Doe",
	}

	if err := runCreate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})
	assignee := fields["assignee"].(map[string]interface{})
	if assignee["accountId"] != "abc123def456abc123def456" {
		t.Errorf("assignee accountId = %v, want abc123def456abc123def456", assignee["accountId"])
	}
}

func TestCreateProjectFromConfig(t *testing.T) {
	f, tio, _ := newTestCreateFactory(t, createHandler(nil), func(cfg config.Config) {
		if err := cfg.Set("default.project", "CONF"); err != nil {
			panic(err)
		}
		cfg.Save()
	})

	opts := &CreateOptions{
		Factory: f,
		Type:    "Bug",
		Summary: "Test from config",
	}

	if err := runCreate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ-124") {
		t.Errorf("output missing issue key: %s", out)
	}
}

func TestCreateAssigneeFromConfig(t *testing.T) {
	var capturedBody string
	f, _, _ := newTestCreateFactory(t, createHandler(&capturedBody), func(cfg config.Config) {
		if err := cfg.Set("default.assignee", "@me"); err != nil {
			panic(err)
		}
		cfg.Save()
	})

	opts := &CreateOptions{
		Factory: f,
		Project: "PROJ",
		Type:    "Bug",
		Summary: "Test with default assignee",
	}

	if err := runCreate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})
	assignee := fields["assignee"].(map[string]interface{})
	if assignee["accountId"] != "abc123def456abc123def456" {
		t.Errorf("assignee accountId = %v, want abc123def456abc123def456", assignee["accountId"])
	}
}

// --- body-file tests ---

func TestCreateBodyFileFromFile(t *testing.T) {
	dir := t.TempDir()
	descFile := filepath.Join(dir, "desc.md")
	if err := os.WriteFile(descFile, []byte("# File Description\n\nFrom file."), 0644); err != nil {
		t.Fatal(err)
	}

	var capturedBody string
	f, _, _ := newTestCreateFactory(t, createHandler(&capturedBody), nil)

	opts := &CreateOptions{
		Factory:  f,
		Project:  "PROJ",
		Type:     "Bug",
		Summary:  "Test with body-file",
		BodyFile: descFile,
	}

	if err := runCreate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})

	// Description should be ADF (from file content).
	desc, ok := fields["description"].(map[string]interface{})
	if !ok {
		t.Fatalf("description should be ADF object, got %T", fields["description"])
	}
	if desc["type"] != "doc" {
		t.Errorf("description type = %v, want 'doc'", desc["type"])
	}
}

func TestCreateBodyFileOverridesDescription(t *testing.T) {
	dir := t.TempDir()
	descFile := filepath.Join(dir, "desc.md")
	if err := os.WriteFile(descFile, []byte("From file"), 0644); err != nil {
		t.Fatal(err)
	}

	var capturedBody string
	f, _, _ := newTestCreateFactory(t, createHandler(&capturedBody), nil)

	opts := &CreateOptions{
		Factory:     f,
		Project:     "PROJ",
		Type:        "Bug",
		Summary:     "Test",
		Description: "From flag (should be overridden)",
		BodyFile:    descFile,
	}

	if err := runCreate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the ADF was generated (body-file content wins over --description).
	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})
	if _, ok := fields["description"]; !ok {
		t.Error("description should be present (from body-file)")
	}
}

func TestCreateBodyFileFromStdin(t *testing.T) {
	var capturedBody string
	f, tio, _ := newTestCreateFactory(t, createHandler(&capturedBody), nil)

	// Write content to the stdin buffer.
	tio.InBuf.WriteString("Description from stdin")

	opts := &CreateOptions{
		Factory:  f,
		Project:  "PROJ",
		Type:     "Bug",
		Summary:  "Test with stdin",
		BodyFile: "-",
	}

	if err := runCreate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})

	desc, ok := fields["description"].(map[string]interface{})
	if !ok {
		t.Fatalf("description should be ADF object, got %T", fields["description"])
	}
	if desc["type"] != "doc" {
		t.Errorf("description type = %v, want 'doc'", desc["type"])
	}
}

func TestCreateBodyFileNotFound(t *testing.T) {
	f, _, _ := newTestCreateFactory(t, createHandler(nil), nil)

	opts := &CreateOptions{
		Factory:  f,
		Project:  "PROJ",
		Type:     "Bug",
		Summary:  "Test",
		BodyFile: "/nonexistent/file.md",
	}

	err := runCreate(opts)
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
	if !strings.Contains(cliErr.Message, "File not found") {
		t.Errorf("error message = %q, want 'File not found'", cliErr.Message)
	}
}

// --- dry-run tests ---

func TestCreateDryRunText(t *testing.T) {
	f, tio, _ := newTestCreateFactory(t, createHandler(nil), nil)
	f.DryRun = true

	opts := &CreateOptions{
		Factory: f,
		Project: "PROJ",
		Type:    "Bug",
		Summary: "Test dry run",
	}

	if err := runCreate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "DRY RUN") {
		t.Errorf("output should contain 'DRY RUN': %s", out)
	}
	if !strings.Contains(out, "PROJ") {
		t.Errorf("output should contain project: %s", out)
	}
	if !strings.Contains(out, "Bug") {
		t.Errorf("output should contain issue type: %s", out)
	}
	if !strings.Contains(out, "Test dry run") {
		t.Errorf("output should contain summary: %s", out)
	}
	if !strings.Contains(out, "passed") {
		t.Errorf("output should contain validation result: %s", out)
	}
}

func TestCreateDryRunJSON(t *testing.T) {
	f, tio, _ := newTestCreateFactory(t, createHandler(nil), nil)
	f.DryRun = true
	f.OutputJSON = true

	opts := &CreateOptions{
		Factory: f,
		Project: "PROJ",
		Type:    "Bug",
		Summary: "Test dry run JSON",
	}

	if err := runCreate(opts); err != nil {
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
	if result["validation"] == nil {
		t.Error("validation should be present")
	}
	validStr, ok := result["validation"].(string)
	if !ok {
		t.Fatalf("validation should be string, got %T", result["validation"])
	}
	if !strings.Contains(validStr, "passed") {
		t.Errorf("validation = %q, want 'passed'", validStr)
	}

	payload, ok := result["payload"].(map[string]interface{})
	if !ok {
		t.Fatalf("payload should be object, got %T", result["payload"])
	}
	if payload["summary"] != "Test dry run JSON" {
		t.Errorf("payload.summary = %v, want 'Test dry run JSON'", payload["summary"])
	}
}

func TestCreateDryRunDoesNotCreate(t *testing.T) {
	created := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/issue" {
			created = true
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(api.CreatedIssue{
				ID:  "10042",
				Key: "PROJ-124",
			})
			return
		}
		// Createmeta endpoint.
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "createmeta") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"issueTypes": []map[string]interface{}{
					{"id": "10001", "name": "Bug", "subtask": false},
				},
			})
			return
		}
		w.WriteHeader(404)
		w.Write([]byte(`{"errorMessages":["Not found"]}`))
	})

	f, _, _ := newTestCreateFactory(t, handler, nil)
	f.DryRun = true

	opts := &CreateOptions{
		Factory: f,
		Project: "PROJ",
		Type:    "Bug",
		Summary: "Test dry run no create",
	}

	if err := runCreate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if created {
		t.Error("dry-run should not call POST /issue")
	}
}

func TestCreateDryRunInvalidType(t *testing.T) {
	f, tio, _ := newTestCreateFactory(t, createHandler(nil), nil)
	f.DryRun = true

	opts := &CreateOptions{
		Factory: f,
		Project: "PROJ",
		Type:    "NonexistentType",
		Summary: "Test dry run bad type",
	}

	if err := runCreate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "warning") || !strings.Contains(out, "NonexistentType") {
		t.Errorf("output should warn about invalid type: %s", out)
	}
}

func TestCreateDryRunWithAssignee(t *testing.T) {
	f, tio, _ := newTestCreateFactory(t, createHandler(nil), nil)
	f.DryRun = true
	f.OutputJSON = true

	opts := &CreateOptions{
		Factory:  f,
		Project:  "PROJ",
		Type:     "Bug",
		Summary:  "Test dry run assignee",
		Assignee: "Jane Doe",
	}

	if err := runCreate(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	payload := result["payload"].(map[string]interface{})
	if _, ok := payload["assignee"]; !ok {
		t.Error("payload should contain resolved assignee")
	}
}

func TestCreateBodyFileWithDryRun(t *testing.T) {
	dir := t.TempDir()
	descFile := filepath.Join(dir, "desc.md")
	if err := os.WriteFile(descFile, []byte("Dry run body from file"), 0644); err != nil {
		t.Fatal(err)
	}

	f, tio, _ := newTestCreateFactory(t, createHandler(nil), nil)
	f.DryRun = true
	f.OutputJSON = true

	opts := &CreateOptions{
		Factory:  f,
		Project:  "PROJ",
		Type:     "Bug",
		Summary:  "Test body-file + dry-run",
		BodyFile: descFile,
	}

	if err := runCreate(opts); err != nil {
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

	payload := result["payload"].(map[string]interface{})
	if _, ok := payload["description"]; !ok {
		t.Error("payload should contain description from body-file")
	}
}
