package issue

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/auth"
	"github.com/endgameio/jira-cli/internal/config"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/iostreams"
)

// newTestEditFactory creates a Factory wired to a test httptest server.
func newTestEditFactory(t *testing.T, handler http.Handler, cfgSetup func(config.Config)) (*factory.Factory, *iostreams.TestIOStreams, *httptest.Server) {
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

// editHandler returns an HTTP handler that serves PUT /issue/{key} responses
// and GET /user/search responses for assignee resolution. It captures the request body.
func editHandler(captureBody *string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// PUT /issue/{key} — edit issue.
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/issue/") {
			if captureBody != nil {
				body, _ := io.ReadAll(r.Body)
				*captureBody = string(body)
			}
			w.WriteHeader(204)
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

// editAndGetHandler serves both PUT /issue/{key} (edit) and GET /issue/{key} (view)
// for tests that need dry-run (which fetches before diffing).
func editAndGetHandler(captureBody *string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// GET /issue/{key} — return mock issue for dry-run diff.
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/issue/") && !strings.Contains(r.URL.Path, "/user/") {
			json.NewEncoder(w).Encode(api.Issue{
				ID:  "10001",
				Key: "PROJ-123",
				Fields: api.IssueFields{
					Summary: "Old title",
					Labels:  []string{"existing-label", "keep-me"},
					Priority: &api.Priority{
						Name: "Medium",
					},
					Assignee: &api.User{
						AccountID:   "old123old456old123old456",
						DisplayName: "Old Assignee",
					},
				},
			})
			return
		}

		// Delegate to the standard edit handler for other paths.
		editHandler(captureBody)(w, r)
	}
}

func TestEditSingleField(t *testing.T) {
	var capturedBody string
	f, tio, _ := newTestEditFactory(t, editHandler(&capturedBody), nil)

	opts := &EditOptions{
		Factory:    f,
		KeyOrID:    "PROJ-123",
		Summary:    "Updated title",
		summarySet: true,
	}

	if err := runEdit(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ-123") {
		t.Errorf("output missing issue key: %s", out)
	}
	if !strings.Contains(out, "Updated") {
		t.Errorf("output missing 'Updated': %s", out)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})
	if fields["summary"] != "Updated title" {
		t.Errorf("summary = %v, want 'Updated title'", fields["summary"])
	}
}

func TestEditMultipleFields(t *testing.T) {
	var capturedBody string
	f, _, _ := newTestEditFactory(t, editHandler(&capturedBody), nil)

	opts := &EditOptions{
		Factory:        f,
		KeyOrID:        "PROJ-123",
		Summary:        "New summary",
		summarySet:     true,
		Description:    "Some **bold** text",
		descriptionSet: true,
		Priority:       "High",
		Labels:         []string{"frontend", "urgent"},
		labelsSet:      true,
	}

	if err := runEdit(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})

	if fields["summary"] != "New summary" {
		t.Errorf("summary = %v, want 'New summary'", fields["summary"])
	}

	pri := fields["priority"].(map[string]interface{})
	if pri["name"] != "High" {
		t.Errorf("priority = %v, want High", pri["name"])
	}

	labels := fields["labels"].([]interface{})
	if len(labels) != 2 || labels[0] != "frontend" || labels[1] != "urgent" {
		t.Errorf("labels = %v, want [frontend urgent]", labels)
	}

	desc, ok := fields["description"].(map[string]interface{})
	if !ok {
		t.Fatalf("description should be ADF object, got %T", fields["description"])
	}
	if desc["type"] != "doc" {
		t.Errorf("description type = %v, want 'doc'", desc["type"])
	}
}

func TestEditUnassign(t *testing.T) {
	var capturedBody string
	f, _, _ := newTestEditFactory(t, editHandler(&capturedBody), nil)

	opts := &EditOptions{
		Factory:     f,
		KeyOrID:     "PROJ-123",
		Assignee:    "",
		assigneeSet: true,
	}

	if err := runEdit(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})

	// assignee should be null (nil in JSON).
	if fields["assignee"] != nil {
		t.Errorf("assignee = %v, want null", fields["assignee"])
	}
}

func TestEditAssignee(t *testing.T) {
	var capturedBody string
	f, _, _ := newTestEditFactory(t, editHandler(&capturedBody), nil)

	opts := &EditOptions{
		Factory:     f,
		KeyOrID:     "PROJ-123",
		Assignee:    "Jane Doe",
		assigneeSet: true,
	}

	if err := runEdit(opts); err != nil {
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

func TestEditNoFieldsError(t *testing.T) {
	f, _, _ := newTestEditFactory(t, editHandler(nil), nil)

	opts := &EditOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
	}

	err := runEdit(opts)
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
	if !strings.Contains(cliErr.Message, "At least one") {
		t.Errorf("error message = %q, want 'At least one field flag'", cliErr.Message)
	}
}

func TestEditEmptySummaryError(t *testing.T) {
	f, _, _ := newTestEditFactory(t, editHandler(nil), nil)

	opts := &EditOptions{
		Factory:    f,
		KeyOrID:    "PROJ-123",
		Summary:    "",
		summarySet: true,
	}

	err := runEdit(opts)
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
	if !strings.Contains(cliErr.Message, "Summary cannot be empty") {
		t.Errorf("error message = %q, want 'Summary cannot be empty'", cliErr.Message)
	}
}

func TestEditClearDescription(t *testing.T) {
	var capturedBody string
	f, _, _ := newTestEditFactory(t, editHandler(&capturedBody), nil)

	opts := &EditOptions{
		Factory:        f,
		KeyOrID:        "PROJ-123",
		Description:    "",
		descriptionSet: true,
	}

	if err := runEdit(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}
	fields := reqBody["fields"].(map[string]interface{})

	// Empty description → empty ADF document.
	desc, ok := fields["description"].(map[string]interface{})
	if !ok {
		t.Fatalf("description should be ADF object, got %T", fields["description"])
	}
	if desc["type"] != "doc" {
		t.Errorf("description type = %v, want 'doc'", desc["type"])
	}
}

func TestEdit404Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"errorMessages":["Issue does not exist or you do not have permission to see it."]}`))
	})

	f, _, _ := newTestEditFactory(t, handler, nil)

	opts := &EditOptions{
		Factory:    f,
		KeyOrID:    "PROJ-999",
		Summary:    "Updated",
		summarySet: true,
	}

	err := runEdit(opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.NOT_FOUND {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.NOT_FOUND)
	}
}

func TestEdit403Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"errorMessages":["You do not have permission to edit issues in this project."]}`))
	})

	f, _, _ := newTestEditFactory(t, handler, nil)

	opts := &EditOptions{
		Factory:    f,
		KeyOrID:    "PROJ-123",
		Summary:    "Updated",
		summarySet: true,
	}

	err := runEdit(opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.PERMISSION_DENIED {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.PERMISSION_DENIED)
	}
}

func TestEdit409Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(409)
		w.Write([]byte(`{"errorMessages":["Conflict"]}`))
	})

	f, _, _ := newTestEditFactory(t, handler, nil)

	opts := &EditOptions{
		Factory:    f,
		KeyOrID:    "PROJ-123",
		Summary:    "Updated",
		summarySet: true,
	}

	err := runEdit(opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.CONFLICT_ERROR {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.CONFLICT_ERROR)
	}
}

func TestEditFieldSplitOnFirstEquals(t *testing.T) {
	var capturedBody string
	f, _, _ := newTestEditFactory(t, editHandler(&capturedBody), nil)

	opts := &EditOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		Fields:  []string{"cf_url=https://example.com?a=1&b=2"},
	}

	if err := runEdit(opts); err != nil {
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

func TestEditFieldCollisionWarning(t *testing.T) {
	f, tio, _ := newTestEditFactory(t, editHandler(nil), nil)

	opts := &EditOptions{
		Factory:    f,
		KeyOrID:    "PROJ-123",
		Summary:    "From named flag",
		summarySet: true,
		Fields:     []string{"summary=Ignored Value"},
	}

	if err := runEdit(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	errOut := tio.ErrBuf.String()
	if !strings.Contains(errOut, "Warning") || !strings.Contains(errOut, "summary") {
		t.Errorf("stderr should contain warning about summary collision:\n%s", errOut)
	}
}

func TestEditJSON(t *testing.T) {
	f, tio, _ := newTestEditFactory(t, editHandler(nil), nil)
	f.OutputJSON = true

	opts := &EditOptions{
		Factory:    f,
		KeyOrID:    "PROJ-123",
		Summary:    "Updated title",
		summarySet: true,
	}

	if err := runEdit(opts); err != nil {
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
	if result["key"] != "PROJ-123" {
		t.Errorf("key = %v, want PROJ-123", result["key"])
	}
	updatedFields, ok := result["updated_fields"].([]interface{})
	if !ok {
		t.Fatalf("updated_fields should be array, got %T", result["updated_fields"])
	}
	if len(updatedFields) == 0 {
		t.Error("updated_fields should not be empty")
	}
}

func TestEditQuiet(t *testing.T) {
	f, tio, _ := newTestEditFactory(t, editHandler(nil), nil)
	f.Quiet = true

	opts := &EditOptions{
		Factory:    f,
		KeyOrID:    "PROJ-123",
		Summary:    "Updated title",
		summarySet: true,
	}

	if err := runEdit(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if out != "" {
		t.Errorf("quiet mode should produce no output, got: %s", out)
	}
}

func TestEditInvalidFieldFormat(t *testing.T) {
	f, _, _ := newTestEditFactory(t, editHandler(nil), nil)

	opts := &EditOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		Fields:  []string{"no-equals-sign"},
	}

	err := runEdit(opts)
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

// --- US-023b: Label operations, field collision, dry-run ---

func TestEditAddLabels(t *testing.T) {
	var capturedBody string
	f, _, _ := newTestEditFactory(t, editHandler(&capturedBody), nil)

	opts := &EditOptions{
		Factory:   f,
		KeyOrID:   "PROJ-123",
		AddLabels: []string{"new-label", "another"},
	}

	if err := runEdit(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}

	// Should have update.labels with add operations.
	updateRaw, ok := reqBody["update"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected update section, got %T", reqBody["update"])
	}
	labelsOps, ok := updateRaw["labels"].([]interface{})
	if !ok {
		t.Fatalf("expected labels ops array, got %T", updateRaw["labels"])
	}
	if len(labelsOps) != 2 {
		t.Fatalf("expected 2 label ops, got %d", len(labelsOps))
	}

	op0 := labelsOps[0].(map[string]interface{})
	if op0["add"] != "new-label" {
		t.Errorf("op[0] = %v, want add:new-label", op0)
	}
	op1 := labelsOps[1].(map[string]interface{})
	if op1["add"] != "another" {
		t.Errorf("op[1] = %v, want add:another", op1)
	}

	// Should NOT have fields.labels (add/remove uses update, not fields).
	if fieldsRaw, ok := reqBody["fields"].(map[string]interface{}); ok {
		if _, hasLabels := fieldsRaw["labels"]; hasLabels {
			t.Error("fields should not contain labels when using --add-labels")
		}
	}
}

func TestEditRemoveLabels(t *testing.T) {
	var capturedBody string
	f, _, _ := newTestEditFactory(t, editHandler(&capturedBody), nil)

	opts := &EditOptions{
		Factory:      f,
		KeyOrID:      "PROJ-123",
		RemoveLabels: []string{"old-label"},
	}

	if err := runEdit(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}

	updateRaw := reqBody["update"].(map[string]interface{})
	labelsOps := updateRaw["labels"].([]interface{})
	if len(labelsOps) != 1 {
		t.Fatalf("expected 1 label op, got %d", len(labelsOps))
	}

	op := labelsOps[0].(map[string]interface{})
	if op["remove"] != "old-label" {
		t.Errorf("op = %v, want remove:old-label", op)
	}
}

func TestEditAddAndRemoveLabels(t *testing.T) {
	var capturedBody string
	f, _, _ := newTestEditFactory(t, editHandler(&capturedBody), nil)

	opts := &EditOptions{
		Factory:      f,
		KeyOrID:      "PROJ-123",
		AddLabels:    []string{"added"},
		RemoveLabels: []string{"removed"},
	}

	if err := runEdit(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]interface{}
	if err := json.Unmarshal([]byte(capturedBody), &reqBody); err != nil {
		t.Fatalf("invalid request JSON: %v", err)
	}

	updateRaw := reqBody["update"].(map[string]interface{})
	labelsOps := updateRaw["labels"].([]interface{})
	if len(labelsOps) != 2 {
		t.Fatalf("expected 2 label ops, got %d", len(labelsOps))
	}

	// First op should be add, second should be remove.
	op0 := labelsOps[0].(map[string]interface{})
	if op0["add"] != "added" {
		t.Errorf("op[0] = %v, want add:added", op0)
	}
	op1 := labelsOps[1].(map[string]interface{})
	if op1["remove"] != "removed" {
		t.Errorf("op[1] = %v, want remove:removed", op1)
	}
}

func TestEditLabelsConflictWithAddLabels(t *testing.T) {
	f, _, _ := newTestEditFactory(t, editHandler(nil), nil)

	opts := &EditOptions{
		Factory:   f,
		KeyOrID:   "PROJ-123",
		Labels:    []string{"replace"},
		labelsSet: true,
		AddLabels: []string{"add-this"},
	}

	err := runEdit(opts)
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
	if !strings.Contains(cliErr.Message, "--labels cannot be combined") {
		t.Errorf("error message = %q, want '--labels cannot be combined...'", cliErr.Message)
	}
}

func TestEditLabelsConflictWithRemoveLabels(t *testing.T) {
	f, _, _ := newTestEditFactory(t, editHandler(nil), nil)

	opts := &EditOptions{
		Factory:      f,
		KeyOrID:      "PROJ-123",
		Labels:       []string{"replace"},
		labelsSet:    true,
		RemoveLabels: []string{"remove-this"},
	}

	err := runEdit(opts)
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

func TestEditDryRunText(t *testing.T) {
	f, tio, _ := newTestEditFactory(t, editAndGetHandler(nil), nil)
	f.DryRun = true

	opts := &EditOptions{
		Factory:    f,
		KeyOrID:    "PROJ-123",
		Summary:    "New title",
		summarySet: true,
	}

	if err := runEdit(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "DRY RUN") {
		t.Errorf("output should contain 'DRY RUN', got: %s", out)
	}
	if !strings.Contains(out, "PROJ-123") {
		t.Errorf("output should contain issue key, got: %s", out)
	}
	if !strings.Contains(out, "Old title") {
		t.Errorf("output should contain 'Old title' (from value), got: %s", out)
	}
	if !strings.Contains(out, "New title") {
		t.Errorf("output should contain 'New title' (to value), got: %s", out)
	}
}

func TestEditDryRunJSON(t *testing.T) {
	f, tio, _ := newTestEditFactory(t, editAndGetHandler(nil), nil)
	f.DryRun = true
	f.OutputJSON = true

	opts := &EditOptions{
		Factory:    f,
		KeyOrID:    "PROJ-123",
		Summary:    "New title",
		summarySet: true,
		Priority:   "High",
	}

	if err := runEdit(opts); err != nil {
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
	if result["validation"] != "passed" {
		t.Errorf("validation = %v, want 'passed'", result["validation"])
	}

	// Payload is the changes array directly (OutputDryRun wraps it).
	changes, ok := result["payload"].([]interface{})
	if !ok {
		t.Fatalf("payload should be array of changes, got %T", result["payload"])
	}
	if len(changes) < 2 {
		t.Fatalf("expected at least 2 changes, got %d", len(changes))
	}
}

func TestEditDryRunDoesNotMutate(t *testing.T) {
	editCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/issue/") {
			editCalled = true
			w.WriteHeader(204)
			return
		}
		// GET /issue/{key} for dry-run.
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/issue/") {
			json.NewEncoder(w).Encode(api.Issue{
				ID:  "10001",
				Key: "PROJ-123",
				Fields: api.IssueFields{
					Summary: "Old title",
				},
			})
			return
		}
		w.WriteHeader(404)
	})

	f, _, _ := newTestEditFactory(t, handler, nil)
	f.DryRun = true

	opts := &EditOptions{
		Factory:    f,
		KeyOrID:    "PROJ-123",
		Summary:    "New title",
		summarySet: true,
	}

	if err := runEdit(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if editCalled {
		t.Error("dry-run should NOT call the edit (PUT) endpoint")
	}
}

func TestEditDryRunLabels(t *testing.T) {
	f, tio, _ := newTestEditFactory(t, editAndGetHandler(nil), nil)
	f.DryRun = true
	f.OutputJSON = true

	opts := &EditOptions{
		Factory:      f,
		KeyOrID:      "PROJ-123",
		AddLabels:    []string{"new-label"},
		RemoveLabels: []string{"existing-label"},
	}

	if err := runEdit(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	// Payload is the changes array directly.
	changes := result["payload"].([]interface{})

	// Find the labels change.
	var labelsChange map[string]interface{}
	for _, c := range changes {
		change := c.(map[string]interface{})
		if change["field"] == "labels" {
			labelsChange = change
			break
		}
	}
	if labelsChange == nil {
		t.Fatal("expected a labels change in dry-run output")
	}

	// "to" should contain resulting labels after add/remove.
	toLabels := labelsChange["to"].([]interface{})
	toStrings := make([]string, len(toLabels))
	for i, l := range toLabels {
		toStrings[i] = l.(string)
	}
	sort.Strings(toStrings)

	// Expected: keep-me + new-label (existing-label removed).
	expected := []string{"keep-me", "new-label"}
	sort.Strings(expected)
	if len(toStrings) != len(expected) {
		t.Fatalf("to labels = %v, want %v", toStrings, expected)
	}
	for i := range expected {
		if toStrings[i] != expected[i] {
			t.Errorf("to labels[%d] = %q, want %q", i, toStrings[i], expected[i])
		}
	}
}

func TestComputeLabelDelta(t *testing.T) {
	tests := []struct {
		name    string
		current []string
		add     []string
		remove  []string
		want    []string
	}{
		{
			name:    "add only",
			current: []string{"a", "b"},
			add:     []string{"c"},
			want:    []string{"a", "b", "c"},
		},
		{
			name:    "remove only",
			current: []string{"a", "b", "c"},
			remove:  []string{"b"},
			want:    []string{"a", "c"},
		},
		{
			name:    "add and remove",
			current: []string{"a", "b"},
			add:     []string{"c"},
			remove:  []string{"a"},
			want:    []string{"b", "c"},
		},
		{
			name:    "add duplicate",
			current: []string{"a"},
			add:     []string{"a"},
			want:    []string{"a"},
		},
		{
			name:    "remove nonexistent",
			current: []string{"a"},
			remove:  []string{"b"},
			want:    []string{"a"},
		},
		{
			name:    "empty current",
			current: nil,
			add:     []string{"x"},
			want:    []string{"x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeLabelDelta(tt.current, tt.add, tt.remove)
			sort.Strings(got)
			sort.Strings(tt.want)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
