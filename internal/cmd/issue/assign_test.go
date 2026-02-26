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
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/iostreams"
)

// sampleUser returns a test user for assign tests.
func sampleUser() api.User {
	email := "jane@example.com"
	return api.User{
		AccountID:    "5b10ac8d82e05b22cc7d4ef5",
		DisplayName:  "Jane Doe",
		EmailAddress: &email,
		Active:       true,
	}
}

// assignHandler returns an HTTP handler for assign-related endpoints.
func assignHandler(issue api.Issue, user api.User, assignErr int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// PUT /issue/{key}/assignee
		if strings.HasSuffix(path, "/assignee") && r.Method == http.MethodPut {
			if assignErr != 0 {
				w.WriteHeader(assignErr)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"Assign failed"},
				})
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// GET /issue/{key}
		if strings.Contains(path, "/issue/") && !strings.Contains(path, "/assignee") && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(issue)
			return
		}

		// GET /user/search
		if strings.Contains(path, "/user/search") && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode([]api.User{user})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestAssignFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams, *httptest.Server) {
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

func TestAssignUserText(t *testing.T) {
	issue := sampleIssue()
	user := sampleUser()

	f, tio, _ := newTestAssignFactory(t, assignHandler(issue, user, 0))

	opts := &AssignOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		UserArg: "Jane Doe",
	}

	err := runAssign(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Assigned PROJ-123 to Jane Doe") {
		t.Errorf("expected assign message, got: %s", out)
	}
}

func TestAssignUserJSON(t *testing.T) {
	issue := sampleIssue()
	user := sampleUser()

	f, tio, _ := newTestAssignFactory(t, assignHandler(issue, user, 0))
	f.OutputJSON = true

	opts := &AssignOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		UserArg: "Jane Doe",
	}

	err := runAssign(opts)
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
	if result["action"] != "assigned" {
		t.Errorf("expected action:assigned, got: %v", result["action"])
	}
	if result["accountId"] != "5b10ac8d82e05b22cc7d4ef5" {
		t.Errorf("expected accountId, got: %v", result["accountId"])
	}
	if result["assignee"] == nil {
		t.Error("expected assignee object in JSON output")
	}
}

func TestUnassignText(t *testing.T) {
	issue := sampleIssue()
	user := sampleUser()

	// Verify PUT /assignee sends null accountId.
	var receivedBody map[string]interface{}
	handler := func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if strings.HasSuffix(path, "/assignee") && r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			json.Unmarshal(body, &receivedBody)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Delegate to default handler for other routes.
		assignHandler(issue, user, 0)(w, r)
	}

	f, tio, _ := newTestAssignFactory(t, http.HandlerFunc(handler))

	opts := &AssignOptions{
		Factory:  f,
		KeyOrID:  "PROJ-123",
		Unassign: true,
	}

	err := runAssign(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Unassigned PROJ-123") {
		t.Errorf("expected unassign message, got: %s", out)
	}

	// Verify null accountId was sent.
	if receivedBody["accountId"] != nil {
		t.Errorf("expected null accountId, got: %v", receivedBody["accountId"])
	}
}

func TestUnassignJSON(t *testing.T) {
	issue := sampleIssue()
	user := sampleUser()

	f, tio, _ := newTestAssignFactory(t, assignHandler(issue, user, 0))
	f.OutputJSON = true

	opts := &AssignOptions{
		Factory:  f,
		KeyOrID:  "PROJ-123",
		Unassign: true,
	}

	err := runAssign(opts)
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
	if result["action"] != "unassigned" {
		t.Errorf("expected action:unassigned, got: %v", result["action"])
	}
}

func TestAssignUnassignConflict(t *testing.T) {
	cmd := NewCmdAssign(factory.NewTestFactory(iostreams.Test().IOStreams, nil, nil))
	cmd.SetArgs([]string{"PROJ-123", "Jane Doe", "--unassign"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --unassign + user arg conflict")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T", err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("expected VALIDATION_ERROR, got: %s", cliErr.Code)
	}
}

func TestAssignNoUserNoUnassign(t *testing.T) {
	cmd := NewCmdAssign(factory.NewTestFactory(iostreams.Test().IOStreams, nil, nil))
	cmd.SetArgs([]string{"PROJ-123"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no user and no --unassign")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T", err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("expected VALIDATION_ERROR, got: %s", cliErr.Code)
	}
}

func TestAssignDryRunText(t *testing.T) {
	issue := sampleIssue()
	user := sampleUser()

	// Verify no PUT /assignee is called during dry-run.
	putCalled := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/assignee") && r.Method == http.MethodPut {
			putCalled = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		assignHandler(issue, user, 0)(w, r)
	}

	f, tio, _ := newTestAssignFactory(t, http.HandlerFunc(handler))
	f.DryRun = true

	opts := &AssignOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		UserArg: "Jane Doe",
	}

	err := runAssign(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if putCalled {
		t.Error("expected no PUT /assignee call during dry-run")
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ-123") {
		t.Errorf("expected issue key in dry-run output, got: %s", out)
	}
	if !strings.Contains(out, "Assign") {
		t.Errorf("expected action in dry-run output, got: %s", out)
	}
}

func TestAssignDryRunJSON(t *testing.T) {
	issue := sampleIssue()
	user := sampleUser()

	f, tio, _ := newTestAssignFactory(t, assignHandler(issue, user, 0))
	f.DryRun = true
	f.OutputJSON = true

	opts := &AssignOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		UserArg: "Jane Doe",
	}

	err := runAssign(opts)
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
		if payload["action"] != "assigned" {
			t.Errorf("expected action:assigned in payload, got: %v", payload["action"])
		}
	} else {
		t.Error("expected payload object in dry-run JSON output")
	}
}

func TestUnassignDryRunText(t *testing.T) {
	issue := sampleIssue()
	user := sampleUser()

	f, tio, _ := newTestAssignFactory(t, assignHandler(issue, user, 0))
	f.DryRun = true

	opts := &AssignOptions{
		Factory:  f,
		KeyOrID:  "PROJ-123",
		Unassign: true,
	}

	err := runAssign(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Unassign") {
		t.Errorf("expected 'Unassign' in dry-run output, got: %s", out)
	}
}

func TestAssignUserNotFound(t *testing.T) {
	issue := sampleIssue()

	handler := func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET /user/search returns empty array.
		if strings.Contains(path, "/user/search") {
			json.NewEncoder(w).Encode([]api.User{})
			return
		}

		// GET /issue/{key}
		if strings.Contains(path, "/issue/") {
			json.NewEncoder(w).Encode(issue)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}

	f, _, _ := newTestAssignFactory(t, http.HandlerFunc(handler))

	opts := &AssignOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		UserArg: "nonexistent",
	}

	err := runAssign(opts)
	if err == nil {
		t.Fatal("expected error for user not found")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T", err)
	}
	if cliErr.Code != clierrors.NOT_FOUND {
		t.Errorf("expected NOT_FOUND, got: %s", cliErr.Code)
	}
}

func TestAssign404(t *testing.T) {
	user := sampleUser()

	handler := func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET /user/search returns user.
		if strings.Contains(path, "/user/search") {
			json.NewEncoder(w).Encode([]api.User{user})
			return
		}

		// PUT /assignee returns 404.
		if strings.HasSuffix(path, "/assignee") && r.Method == http.MethodPut {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"Issue does not exist"},
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}

	f, _, _ := newTestAssignFactory(t, http.HandlerFunc(handler))

	opts := &AssignOptions{
		Factory: f,
		KeyOrID: "NONEXIST-999",
		UserArg: "Jane Doe",
	}

	err := runAssign(opts)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestAssign403(t *testing.T) {
	user := sampleUser()

	handler := func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET /user/search returns user.
		if strings.Contains(path, "/user/search") {
			json.NewEncoder(w).Encode([]api.User{user})
			return
		}

		// PUT /assignee returns 403.
		if strings.HasSuffix(path, "/assignee") && r.Method == http.MethodPut {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"errorMessages": []string{"You do not have permission"},
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}

	f, _, _ := newTestAssignFactory(t, http.HandlerFunc(handler))

	opts := &AssignOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		UserArg: "Jane Doe",
	}

	err := runAssign(opts)
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

func TestAssignQuiet(t *testing.T) {
	issue := sampleIssue()
	user := sampleUser()

	f, tio, _ := newTestAssignFactory(t, assignHandler(issue, user, 0))
	f.Quiet = true

	opts := &AssignOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		UserArg: "Jane Doe",
	}

	err := runAssign(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tio.OutBuf.Len() > 0 {
		t.Errorf("expected no output in quiet mode, got: %s", tio.OutBuf.String())
	}
}
