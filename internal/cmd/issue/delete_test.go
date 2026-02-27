package issue

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/auth"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/iostreams"
)

// deleteHandler returns an HTTP handler for delete-related endpoints.
func deleteHandler(issue api.Issue, deleteErr int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// DELETE /issue/{key}
		if r.Method == http.MethodDelete && strings.Contains(path, "/issue/") {
			if deleteErr != 0 {
				w.WriteHeader(deleteErr)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"Delete failed"},
				})
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// GET /issue/{key} (for dry-run validation)
		if r.Method == http.MethodGet && strings.Contains(path, "/issue/") {
			json.NewEncoder(w).Encode(issue)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestDeleteFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams, *httptest.Server) {
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

func TestDeleteSuccessText(t *testing.T) {
	issue := sampleIssue()

	f, tio, _ := newTestDeleteFactory(t, deleteHandler(issue, 0))

	opts := &DeleteOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		Yes:     true,
	}

	err := runDelete(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Deleted PROJ-123") {
		t.Errorf("expected delete message, got: %s", out)
	}
}

func TestDeleteSuccessJSON(t *testing.T) {
	issue := sampleIssue()

	f, tio, _ := newTestDeleteFactory(t, deleteHandler(issue, 0))
	f.OutputJSON = true

	opts := &DeleteOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		Yes:     true,
	}

	err := runDelete(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}

	if result["ok"] != true {
		t.Errorf("expected ok:true, got: %v", result["ok"])
	}
	if result["key"] != "PROJ-123" {
		t.Errorf("expected key:PROJ-123, got: %v", result["key"])
	}
	if result["action"] != "deleted" {
		t.Errorf("expected action:deleted, got: %v", result["action"])
	}
}

func TestDeleteMissingYes(t *testing.T) {
	cmd := NewCmdDelete(factory.NewTestFactory(iostreams.Test().IOStreams, nil, nil))
	cmd.SetArgs([]string{"PROJ-123"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --yes flag")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T", err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("expected VALIDATION_ERROR, got: %s", cliErr.Code)
	}
	if !strings.Contains(cliErr.Message, "--yes") {
		t.Errorf("expected message mentioning --yes, got: %s", cliErr.Message)
	}
}

func TestDelete404(t *testing.T) {
	f, _, _ := newTestDeleteFactory(t, deleteHandler(sampleIssue(), http.StatusNotFound))

	opts := &DeleteOptions{
		Factory: f,
		KeyOrID: "NONEXIST-999",
		Yes:     true,
	}

	err := runDelete(opts)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestDelete403(t *testing.T) {
	f, _, _ := newTestDeleteFactory(t, deleteHandler(sampleIssue(), http.StatusForbidden))

	opts := &DeleteOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		Yes:     true,
	}

	err := runDelete(opts)
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

func TestDeleteDryRunBypassesYes(t *testing.T) {
	// --dry-run should work without --yes. The RunE check should allow
	// the command to proceed when Factory.DryRun is true even without --yes.
	issue := sampleIssue()

	deleteCalled := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		deleteHandler(issue, 0)(w, r)
	}

	f, tio, _ := newTestDeleteFactory(t, http.HandlerFunc(handler))
	f.DryRun = true

	// Exercise the full command path (RunE) without --yes flag.
	cmd := NewCmdDelete(f)
	cmd.SetArgs([]string{"PROJ-123"}) // no --yes

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if deleteCalled {
		t.Error("expected no DELETE call during dry-run")
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ-123") {
		t.Errorf("expected issue key in dry-run output, got: %s", out)
	}
}

func TestDeleteDryRunText(t *testing.T) {
	issue := sampleIssue()

	// Verify no DELETE is called during dry-run.
	deleteCalled := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		deleteHandler(issue, 0)(w, r)
	}

	f, tio, _ := newTestDeleteFactory(t, http.HandlerFunc(handler))
	f.DryRun = true

	opts := &DeleteOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		Yes:     true,
	}

	err := runDelete(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if deleteCalled {
		t.Error("expected no DELETE call during dry-run")
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ-123") {
		t.Errorf("expected issue key in dry-run output, got: %s", out)
	}
	if !strings.Contains(out, "Delete") {
		t.Errorf("expected action in dry-run output, got: %s", out)
	}
}

func TestDeleteDryRunJSON(t *testing.T) {
	issue := sampleIssue()

	f, tio, _ := newTestDeleteFactory(t, deleteHandler(issue, 0))
	f.DryRun = true
	f.OutputJSON = true

	opts := &DeleteOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		Yes:     true,
	}

	err := runDelete(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}

	if result["dry_run"] != true {
		t.Errorf("expected dry_run:true, got: %v", result["dry_run"])
	}
	if payload, ok := result["payload"].(map[string]interface{}); ok {
		if payload["action"] != "deleted" {
			t.Errorf("expected action:deleted in payload, got: %v", payload["action"])
		}
		if payload["deleteSubtasks"] != true {
			t.Errorf("expected deleteSubtasks:true in payload, got: %v", payload["deleteSubtasks"])
		}
	} else {
		t.Error("expected payload object in dry-run JSON output")
	}
}

func TestDeleteQuiet(t *testing.T) {
	issue := sampleIssue()

	f, tio, _ := newTestDeleteFactory(t, deleteHandler(issue, 0))
	f.Quiet = true

	opts := &DeleteOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		Yes:     true,
	}

	err := runDelete(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tio.OutBuf.Len() > 0 {
		t.Errorf("expected no output in quiet mode, got: %s", tio.OutBuf.String())
	}
}

func TestDeleteSubtasksQueryParam(t *testing.T) {
	var gotPath string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			gotPath = r.URL.RequestURI()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	f, _, _ := newTestDeleteFactory(t, handler)

	opts := &DeleteOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		Yes:     true,
	}

	err := runDelete(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(gotPath, "deleteSubtasks=true") {
		t.Errorf("expected deleteSubtasks=true in path, got: %s", gotPath)
	}
}

func TestDeleteInvalidKey(t *testing.T) {
	cmd := NewCmdDelete(factory.NewTestFactory(iostreams.Test().IOStreams, nil, nil))
	cmd.SetArgs([]string{"invalid key!", "--yes"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid key")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T", err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("expected VALIDATION_ERROR, got: %s", cliErr.Code)
	}
}
