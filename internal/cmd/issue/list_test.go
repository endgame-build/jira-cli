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

// searchRequest captures the decoded request body from POST /search/jql.
type searchRequest struct {
	JQL        string   `json:"jql"`
	Fields     []string `json:"fields"`
	MaxResults int      `json:"maxResults"`
}

// searchHandler returns an HTTP handler that captures the search request
// and returns the given issues.
func searchHandler(t *testing.T, issues []api.Issue, capturedJQL *string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// POST /search/jql
		if r.Method == http.MethodPost && strings.HasSuffix(path, "/search/jql") {
			var req searchRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode search request: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if capturedJQL != nil {
				*capturedJQL = req.JQL
			}

			resp := api.SearchResults{
				Issues: issues,
				IsLast: true,
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// GET /myself (for @me resolution)
		if r.Method == http.MethodGet && strings.HasSuffix(path, "/myself") {
			json.NewEncoder(w).Encode(api.User{
				AccountID:   "myself-id",
				DisplayName: "Current User",
			})
			return
		}

		// GET /user/search (for assignee resolution)
		if r.Method == http.MethodGet && strings.Contains(path, "/user/search") {
			json.NewEncoder(w).Encode([]api.User{
				{AccountID: "resolved-id", DisplayName: "Jane Doe"},
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestListFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams, *httptest.Server) {
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

func sampleIssues() []api.Issue {
	return []api.Issue{
		{
			ID:  "10001",
			Key: "PROJ-1",
			Fields: api.IssueFields{
				Summary: "Fix login bug",
				Status: &api.Status{
					ID:   "3",
					Name: "In Progress",
					StatusCategory: &api.StatusCategory{
						Key: "indeterminate",
					},
				},
				Assignee:  &api.User{AccountID: "abc123", DisplayName: "Jane Doe"},
				Priority:  &api.Priority{ID: "2", Name: "High"},
				IssueType: &api.IssueType{ID: "10001", Name: "Bug"},
			},
		},
		{
			ID:  "10002",
			Key: "PROJ-2",
			Fields: api.IssueFields{
				Summary: "Add dark mode support for the main dashboard page and all sub-pages in the application",
				Status: &api.Status{
					ID:   "1",
					Name: "To Do",
					StatusCategory: &api.StatusCategory{
						Key: "new",
					},
				},
				Priority:  &api.Priority{ID: "3", Name: "Medium"},
				IssueType: &api.IssueType{ID: "10002", Name: "Story"},
			},
		},
	}
}

func TestListDefaultToMine(t *testing.T) {
	var capturedJQL string
	issues := sampleIssues()

	f, tio, _ := newTestListFactory(t, searchHandler(t, issues, &capturedJQL))

	opts := &ListOptions{
		Factory: f,
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No filters → default JQL.
	expected := "assignee = currentUser() AND resolution = Unresolved"
	if capturedJQL != expected {
		t.Errorf("JQL mismatch:\n  got:  %s\n  want: %s", capturedJQL, expected)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ-1") {
		t.Errorf("expected PROJ-1 in output, got: %s", out)
	}
	if !strings.Contains(out, "PROJ-2") {
		t.Errorf("expected PROJ-2 in output, got: %s", out)
	}
}

func TestListFilterComposition(t *testing.T) {
	var capturedJQL string

	f, _, _ := newTestListFactory(t, searchHandler(t, sampleIssues(), &capturedJQL))

	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
		Status:  "In Progress",
		Type:    "Bug",
		Label:   "frontend",
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All filter flags joined with AND.
	for _, clause := range []string{
		`project = "PROJ"`,
		`status = "In Progress"`,
		`issuetype = "Bug"`,
		`labels = "frontend"`,
	} {
		if !strings.Contains(capturedJQL, clause) {
			t.Errorf("missing clause %q in JQL: %s", clause, capturedJQL)
		}
	}

	// Check they're joined with AND.
	if !strings.Contains(capturedJQL, " AND ") {
		t.Errorf("expected AND joins in JQL: %s", capturedJQL)
	}
}

func TestListAssigneeAtMe(t *testing.T) {
	var capturedJQL string

	f, _, _ := newTestListFactory(t, searchHandler(t, sampleIssues(), &capturedJQL))

	opts := &ListOptions{
		Factory:  f,
		Assignee: "@me",
		Limit:    50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(capturedJQL, "assignee = currentUser()") {
		t.Errorf("expected currentUser() in JQL, got: %s", capturedJQL)
	}
}

func TestListAssigneeResolved(t *testing.T) {
	var capturedJQL string

	f, _, _ := newTestListFactory(t, searchHandler(t, sampleIssues(), &capturedJQL))

	opts := &ListOptions{
		Factory:  f,
		Assignee: "jane",
		Limit:    50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should contain resolved account ID from the mock.
	if !strings.Contains(capturedJQL, `assignee = "resolved-id"`) {
		t.Errorf("expected resolved account ID in JQL, got: %s", capturedJQL)
	}
}

func TestListJQLOverridesFilters(t *testing.T) {
	var capturedJQL string

	f, _, _ := newTestListFactory(t, searchHandler(t, sampleIssues(), &capturedJQL))

	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",  // should be ignored
		Status:  "To Do", // should be ignored
		JQL:     "reporter = currentUser() ORDER BY created DESC",
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// --jql overrides all filter flags.
	if capturedJQL != "reporter = currentUser() ORDER BY created DESC" {
		t.Errorf("--jql should override filters, got: %s", capturedJQL)
	}
}

func TestListEmptyResults(t *testing.T) {
	f, tio, _ := newTestListFactory(t, searchHandler(t, []api.Issue{}, nil))

	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "No issues found") {
		t.Errorf("expected 'No issues found', got: %s", out)
	}
}

func TestListEmptyResultsJSON(t *testing.T) {
	f, tio, _ := newTestListFactory(t, searchHandler(t, []api.Issue{}, nil))
	f.OutputJSON = true

	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	// JSON envelope should have empty data array.
	var envelope struct {
		Data       []interface{} `json:"data"`
		Pagination struct {
			Offset      int  `json:"offset"`
			Limit       int  `json:"limit"`
			HasNextPage bool `json:"has_next_page"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON: %v\noutput: %s", err, out)
	}
	if len(envelope.Data) != 0 {
		t.Errorf("expected empty data array, got %d items", len(envelope.Data))
	}
}

func TestListJSONOutput(t *testing.T) {
	issues := sampleIssues()
	f, tio, _ := newTestListFactory(t, searchHandler(t, issues, nil))
	f.OutputJSON = true

	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var envelope struct {
		Data       []api.Issue `json:"data"`
		Pagination struct {
			Offset      int  `json:"offset"`
			Limit       int  `json:"limit"`
			HasNextPage bool `json:"has_next_page"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if len(envelope.Data) != 2 {
		t.Errorf("expected 2 issues, got %d", len(envelope.Data))
	}
	if envelope.Data[0].Key != "PROJ-1" {
		t.Errorf("expected PROJ-1, got %s", envelope.Data[0].Key)
	}
	if envelope.Pagination.Limit != 50 {
		t.Errorf("expected limit 50, got %d", envelope.Pagination.Limit)
	}
}

func TestListQuietMode(t *testing.T) {
	f, tio, _ := newTestListFactory(t, searchHandler(t, sampleIssues(), nil))
	f.Quiet = true

	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tio.OutBuf.String() != "" {
		t.Errorf("expected no output in quiet mode, got: %s", tio.OutBuf.String())
	}
}

func TestListSummaryTruncation(t *testing.T) {
	issues := sampleIssues()
	f, tio, _ := newTestListFactory(t, searchHandler(t, issues, nil))

	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	// Second issue has a >60 char summary, should be truncated with "..."
	if !strings.Contains(out, "...") {
		t.Errorf("expected truncated summary with '...', got: %s", out)
	}
}

func TestListTextShowsUnassigned(t *testing.T) {
	issues := sampleIssues()
	// Second issue has no assignee.
	f, tio, _ := newTestListFactory(t, searchHandler(t, issues, nil))

	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Unassigned") {
		t.Errorf("expected 'Unassigned' for nil assignee, got: %s", out)
	}
}

func TestListPagination(t *testing.T) {
	// Mock returns issues with hasNextPage indicator.
	callCount := 0
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jql") {
			callCount++
			resp := api.SearchResults{
				Issues:        sampleIssues(),
				NextPageToken: "next-token",
				IsLast:        false,
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}

	f, tio, _ := newTestListFactory(t, http.HandlerFunc(handler))
	f.OutputJSON = true

	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
		Limit:   2,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var envelope struct {
		Pagination struct {
			HasNextPage bool `json:"has_next_page"`
			Limit       int  `json:"limit"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if !envelope.Pagination.HasNextPage {
		t.Error("expected has_next_page=true")
	}
	if envelope.Pagination.Limit != 2 {
		t.Errorf("expected limit=2, got %d", envelope.Pagination.Limit)
	}
}

func TestListFieldsIndependentOfJQL(t *testing.T) {
	// Capture the request to verify fields are sent even with --jql.
	var capturedFields []string
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jql") {
			var req searchRequest
			json.NewDecoder(r.Body).Decode(&req)
			capturedFields = req.Fields

			json.NewEncoder(w).Encode(api.SearchResults{
				Issues: sampleIssues(),
				IsLast: true,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}

	f, _, _ := newTestListFactory(t, http.HandlerFunc(handler))

	opts := &ListOptions{
		Factory: f,
		JQL:     "project = PROJ",
		Fields:  []string{"summary", "status"},
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// --fields should be sent as API fields parameter even with --jql.
	if len(capturedFields) != 2 || capturedFields[0] != "summary" || capturedFields[1] != "status" {
		t.Errorf("expected fields [summary, status], got: %v", capturedFields)
	}
}

func TestBuildJQLProjectFromConfig(t *testing.T) {
	// Test that buildJQL uses default.project from config when --project is empty.
	var capturedJQL string

	f, _, _ := newTestListFactory(t, searchHandler(t, []api.Issue{}, &capturedJQL))

	// NewTestFactory with nil config means configGet returns "".
	// Without project or any filter, it should default to "my open issues".
	opts := &ListOptions{
		Factory: f,
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "assignee = currentUser() AND resolution = Unresolved"
	if capturedJQL != expected {
		t.Errorf("expected default JQL %q, got: %s", expected, capturedJQL)
	}
}

func TestListSortOrder(t *testing.T) {
	var capturedJQL string

	f, _, _ := newTestListFactory(t, searchHandler(t, sampleIssues(), &capturedJQL))

	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
		Sort:    "created",
		Order:   "desc",
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasSuffix(capturedJQL, "ORDER BY created DESC") {
		t.Errorf("expected ORDER BY created DESC suffix, got: %s", capturedJQL)
	}
}

func TestListSortOrderAsc(t *testing.T) {
	var capturedJQL string

	f, _, _ := newTestListFactory(t, searchHandler(t, sampleIssues(), &capturedJQL))

	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
		Sort:    "priority",
		Order:   "asc",
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasSuffix(capturedJQL, "ORDER BY priority ASC") {
		t.Errorf("expected ORDER BY priority ASC suffix, got: %s", capturedJQL)
	}
}

func TestListSortDefaultOrder(t *testing.T) {
	// --sort without explicit --order defaults to desc.
	var capturedJQL string

	f, _, _ := newTestListFactory(t, searchHandler(t, sampleIssues(), &capturedJQL))

	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
		Sort:    "updated",
		Order:   "desc", // default
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasSuffix(capturedJQL, "ORDER BY updated DESC") {
		t.Errorf("expected ORDER BY updated DESC suffix, got: %s", capturedJQL)
	}
}

func TestListOrderWithoutSortError(t *testing.T) {
	f, _, _ := newTestListFactory(t, searchHandler(t, sampleIssues(), nil))

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{"--order", "asc"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --order without --sort")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T %v", err, err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("expected VALIDATION_ERROR, got: %s", cliErr.Code)
	}
	if !strings.Contains(cliErr.Message, "--order requires --sort") {
		t.Errorf("expected '--order requires --sort' in message, got: %s", cliErr.Message)
	}
}

func TestListSortIgnoredWithJQL(t *testing.T) {
	var capturedJQL string

	f, _, _ := newTestListFactory(t, searchHandler(t, sampleIssues(), &capturedJQL))

	opts := &ListOptions{
		Factory: f,
		JQL:     "project = PROJ",
		Sort:    "created", // should be ignored because --jql is set
		Order:   "asc",
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// --jql overrides everything, including --sort/--order.
	if capturedJQL != "project = PROJ" {
		t.Errorf("--sort should be ignored with --jql, got: %s", capturedJQL)
	}
}

func TestListAssigneeNotFound(t *testing.T) {
	// Mock that returns empty user search results.
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/user/search") {
			json.NewEncoder(w).Encode([]api.User{})
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jql") {
			json.NewEncoder(w).Encode(api.SearchResults{IsLast: true})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}

	f, _, _ := newTestListFactory(t, http.HandlerFunc(handler))

	opts := &ListOptions{
		Factory:  f,
		Assignee: "nonexistent",
		Limit:    50,
	}

	err := runList(opts)
	if err == nil {
		t.Fatal("expected error for unresolvable assignee")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T %v", err, err)
	}
	if cliErr.Code != clierrors.NOT_FOUND {
		t.Errorf("expected NOT_FOUND, got: %s", cliErr.Code)
	}
}

func TestListBadAuthOnEmptyResults(t *testing.T) {
	// When search returns empty results and /myself returns 401,
	// the CLI should surface the auth error instead of "No issues found".
	handler := func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if r.Method == http.MethodPost && strings.HasSuffix(path, "/search/jql") {
			json.NewEncoder(w).Encode(api.SearchResults{
				Issues: []api.Issue{},
				IsLast: true,
			})
			return
		}

		if r.Method == http.MethodGet && strings.HasSuffix(path, "/myself") {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"errorMessages":["Client must be authenticated"]}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}

	f, _, _ := newTestListFactory(t, http.HandlerFunc(handler))

	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
		Limit:   50,
	}

	err := runList(opts)
	if err == nil {
		t.Fatal("expected auth error, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T %v", err, err)
	}
	if cliErr.Code != clierrors.AUTH_ERROR {
		t.Errorf("expected AUTH_ERROR, got: %s", cliErr.Code)
	}
}

func TestListTransientProbeFailureDoesNotMaskEmptyResults(t *testing.T) {
	// When search returns empty results and /myself returns 500 (transient),
	// the CLI should NOT propagate the error — it should show "No issues found".
	handler := func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if r.Method == http.MethodPost && strings.HasSuffix(path, "/search/jql") {
			json.NewEncoder(w).Encode(api.SearchResults{
				Issues: []api.Issue{},
				IsLast: true,
			})
			return
		}

		if r.Method == http.MethodGet && strings.HasSuffix(path, "/myself") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}

	f, tio, _ := newTestListFactory(t, http.HandlerFunc(handler))

	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
		Limit:   50,
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("expected no error for transient probe failure, got: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "No issues found") {
		t.Errorf("expected 'No issues found', got: %s", out)
	}

	// Stderr should contain the probe failure warning.
	errOut := tio.ErrBuf.String()
	if !strings.Contains(errOut, "Warning: credential check failed") {
		t.Errorf("expected stderr warning about probe failure, got: %s", errOut)
	}
}

func TestListAssigneeAmbiguous(t *testing.T) {
	// Mock that returns multiple user search results.
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/user/search") {
			json.NewEncoder(w).Encode([]api.User{
				{AccountID: "user1", DisplayName: "Jane Doe"},
				{AccountID: "user2", DisplayName: "Jane Smith"},
			})
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jql") {
			json.NewEncoder(w).Encode(api.SearchResults{IsLast: true})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}

	f, _, _ := newTestListFactory(t, http.HandlerFunc(handler))

	opts := &ListOptions{
		Factory:  f,
		Assignee: "jane",
		Limit:    50,
	}

	err := runList(opts)
	if err == nil {
		t.Fatal("expected error for ambiguous assignee")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T %v", err, err)
	}
	if cliErr.Code != clierrors.AMBIGUOUS_USER {
		t.Errorf("expected AMBIGUOUS_USER, got: %s", cliErr.Code)
	}
}

func TestListFieldsJSONFiltering(t *testing.T) {
	// End-to-end: --fields summary --json should produce JSON where each
	// item only has id, key, self, and fields.summary.
	issues := sampleIssues()
	f, tio, _ := newTestListFactory(t, searchHandler(t, issues, nil))
	f.OutputJSON = true

	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
		Fields:  []string{"summary"},
		Limit:   50,
	}

	if err := runList(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var envelope struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON: %v\noutput: %s", err, out)
	}
	if len(envelope.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(envelope.Data))
	}

	item := envelope.Data[0]
	// Always-included top-level fields.
	if item["key"] != "PROJ-1" {
		t.Errorf("key = %v, want PROJ-1", item["key"])
	}
	if item["id"] == nil {
		t.Error("id should be present")
	}
	if item["self"] == nil {
		t.Error("self should be present")
	}

	// Fields should only contain summary.
	fields, ok := item["fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("fields should be an object, got %T", item["fields"])
	}
	if fields["summary"] == nil {
		t.Error("fields.summary should be present")
	}
	if fields["status"] != nil {
		t.Error("fields.status should NOT be present when not in --fields")
	}
	if fields["assignee"] != nil {
		t.Error("fields.assignee should NOT be present when not in --fields")
	}
	if fields["priority"] != nil {
		t.Error("fields.priority should NOT be present when not in --fields")
	}
}
