package issue

import (
	"encoding/json"
	"errors"
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

// sampleIssue returns a fully populated Issue for tests.
func sampleIssue() api.Issue {
	return api.Issue{
		ID:   "10001",
		Key:  "PROJ-123",
		Self: "https://mysite.atlassian.net/rest/api/3/issue/10001",
		Fields: api.IssueFields{
			Summary: "Fix login bug",
			Description: json.RawMessage(`{
				"type": "doc",
				"version": 1,
				"content": [
					{
						"type": "paragraph",
						"content": [
							{"type": "text", "text": "The login page crashes when the user clicks submit."}
						]
					}
				]
			}`),
			Status: &api.Status{
				ID:   "3",
				Name: "In Progress",
				StatusCategory: &api.StatusCategory{
					ID:  4,
					Key: "indeterminate",
				},
			},
			IssueType: &api.IssueType{ID: "10001", Name: "Bug"},
			Priority:  &api.Priority{ID: "2", Name: "High"},
			Assignee: &api.User{
				AccountID:   "abc123",
				DisplayName: "Jane Doe",
			},
			Reporter: &api.User{
				AccountID:   "def456",
				DisplayName: "John Smith",
			},
			Labels:  []string{"frontend", "urgent"},
			Created: "2026-01-15T10:30:00.000+0000",
			Updated: "2026-02-20T14:00:00.000+0000",
		},
	}
}

// issueHandler returns an HTTP handler that serves a JSON issue or 404.
func issueHandler(issue api.Issue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Path arrives as /issue/PROJ-123 (WithBaseURL replaces full base URL).
		if !strings.HasPrefix(r.URL.Path, "/issue/") {
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(issue)
	}
}

// newTestViewFactory creates a Factory wired to a test httptest server.
func newTestViewFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams, *httptest.Server) {
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

func TestViewSuccessful(t *testing.T) {
	issue := sampleIssue()
	f, tio, _ := newTestViewFactory(t, issueHandler(issue))

	opts := &ViewOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
	}

	if err := runView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()

	// Verify all key fields are present.
	for _, want := range []string{
		"PROJ-123",
		"Fix login bug",
		"In Progress",
		"Bug",
		"High",
		"Jane Doe",
		"John Smith",
		"frontend, urgent",
		"2026-01-15",
		"2026-02-20",
		"login page crashes",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot: %s", want, out)
		}
	}
}

func TestViewFieldFiltering(t *testing.T) {
	issue := sampleIssue()
	f, tio, _ := newTestViewFactory(t, issueHandler(issue))

	opts := &ViewOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		Fields:  []string{"key", "summary"},
	}

	if err := runView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()

	// Key and summary should be present.
	if !strings.Contains(out, "PROJ-123") {
		t.Errorf("output missing key: %s", out)
	}
	if !strings.Contains(out, "Fix login bug") {
		t.Errorf("output missing summary: %s", out)
	}

	// Other fields should NOT be present.
	if strings.Contains(out, "Jane Doe") {
		t.Errorf("output should not contain assignee when filtered: %s", out)
	}
	if strings.Contains(out, "John Smith") {
		t.Errorf("output should not contain reporter when filtered: %s", out)
	}
	if strings.Contains(out, "login page crashes") {
		t.Errorf("output should not contain description when filtered: %s", out)
	}
}

func TestViewNotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"errorMessages":["Issue does not exist"]}`))
	})

	f, _, _ := newTestViewFactory(t, handler)

	opts := &ViewOptions{
		Factory: f,
		KeyOrID: "PROJ-999",
	}

	err := runView(opts)
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

func TestViewJSON(t *testing.T) {
	issue := sampleIssue()
	f, tio, _ := newTestViewFactory(t, issueHandler(issue))
	f.OutputJSON = true

	opts := &ViewOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
	}

	if err := runView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	// Should be a bare issue object (not wrapped).
	if result["key"] != "PROJ-123" {
		t.Errorf("key = %v, want PROJ-123", result["key"])
	}
	if result["id"] != "10001" {
		t.Errorf("id = %v, want 10001", result["id"])
	}
	if _, ok := result["fields"]; !ok {
		t.Error("missing 'fields' in JSON output")
	}
}

func TestViewDescriptionTruncation(t *testing.T) {
	// Build an ADF document with 7 paragraphs.
	var content []interface{}
	for i := 1; i <= 7; i++ {
		content = append(content, map[string]interface{}{
			"type": "paragraph",
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": strings.Repeat("x", 20),
				},
			},
		})
	}
	descJSON, _ := json.Marshal(map[string]interface{}{
		"type":    "doc",
		"version": 1,
		"content": content,
	})

	issue := api.Issue{
		ID:  "10002",
		Key: "PROJ-456",
		Fields: api.IssueFields{
			Summary:     "Long description test",
			Description: json.RawMessage(descJSON),
			Status:      &api.Status{ID: "1", Name: "Open"},
		},
	}

	f, tio, _ := newTestViewFactory(t, issueHandler(issue))

	opts := &ViewOptions{
		Factory: f,
		KeyOrID: "PROJ-456",
	}

	if err := runView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "... (truncated)") {
		t.Errorf("output should indicate truncation for long descriptions: %s", out)
	}
}

func TestViewUnassigned(t *testing.T) {
	issue := sampleIssue()
	issue.Fields.Assignee = nil

	f, tio, _ := newTestViewFactory(t, issueHandler(issue))

	opts := &ViewOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
	}

	if err := runView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Unassigned") {
		t.Errorf("output should show 'Unassigned' for nil assignee: %s", out)
	}
}

func TestViewNoPager(t *testing.T) {
	issue := sampleIssue()
	f, tio, _ := newTestViewFactory(t, issueHandler(issue))

	opts := &ViewOptions{
		Factory: f,
		KeyOrID: "PROJ-123",
		NoPager: true,
	}

	// NoPager is set on IOStreams in RunE, so simulate it here.
	f.IOStreams.NoPager = true

	if err := runView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ-123") {
		t.Errorf("output should still show issue with --no-pager: %s", out)
	}
	_ = tio
}

func TestViewKeyValidation(t *testing.T) {
	cmd := NewCmdView(&factory.Factory{IOStreams: iostreams.Test().IOStreams})
	cmd.SetArgs([]string{"!!invalid!!"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid key, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.VALIDATION_ERROR)
	}
}
