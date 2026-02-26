package issue

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
