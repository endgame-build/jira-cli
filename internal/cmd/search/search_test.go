package search

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

		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestSearchFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams, *httptest.Server) {
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

func TestSearchRawJQL(t *testing.T) {
	var capturedJQL string
	issues := sampleIssues()

	f, tio, _ := newTestSearchFactory(t, searchHandler(t, issues, &capturedJQL))

	opts := &SearchOptions{
		Factory: f,
		JQL:     "project = PROJ ORDER BY created DESC",
		Limit:   50,
	}

	err := runSearch(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Raw JQL passed through directly.
	if capturedJQL != "project = PROJ ORDER BY created DESC" {
		t.Errorf("JQL mismatch:\n  got:  %s\n  want: project = PROJ ORDER BY created DESC", capturedJQL)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ-1") {
		t.Errorf("expected PROJ-1 in output, got: %s", out)
	}
	if !strings.Contains(out, "PROJ-2") {
		t.Errorf("expected PROJ-2 in output, got: %s", out)
	}
}

func TestSearchMine(t *testing.T) {
	var capturedJQL string
	issues := sampleIssues()

	f, tio, _ := newTestSearchFactory(t, searchHandler(t, issues, &capturedJQL))

	opts := &SearchOptions{
		Factory: f,
		Mine:    true,
		Limit:   50,
	}

	err := runSearch(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "assignee = currentUser() AND resolution = Unresolved"
	if capturedJQL != expected {
		t.Errorf("JQL mismatch:\n  got:  %s\n  want: %s", capturedJQL, expected)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ-1") {
		t.Errorf("expected PROJ-1 in output, got: %s", out)
	}
}

func TestSearchMineWithStatus(t *testing.T) {
	var capturedJQL string

	f, _, _ := newTestSearchFactory(t, searchHandler(t, sampleIssues(), &capturedJQL))

	opts := &SearchOptions{
		Factory: f,
		Mine:    true,
		Status:  "In Progress",
		Limit:   50,
	}

	err := runSearch(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := `assignee = currentUser() AND resolution = Unresolved AND status = "In Progress"`
	if capturedJQL != expected {
		t.Errorf("JQL mismatch:\n  got:  %s\n  want: %s", capturedJQL, expected)
	}
}

func TestSearchJQLOverridesMine(t *testing.T) {
	var capturedJQL string

	f, _, _ := newTestSearchFactory(t, searchHandler(t, sampleIssues(), &capturedJQL))

	opts := &SearchOptions{
		Factory: f,
		JQL:     "reporter = currentUser()",
		Mine:    true,   // should be ignored when JQL is set
		Status:  "Done", // should be appended to JQL
		Limit:   50,
	}

	err := runSearch(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Positional JQL overrides --mine, but --status is still applied.
	expected := `(reporter = currentUser()) AND status = "Done"`
	if capturedJQL != expected {
		t.Errorf("JQL should include status filter, got: %s", capturedJQL)
	}
}

func TestSearchNoArgsError(t *testing.T) {
	f, _, _ := newTestSearchFactory(t, searchHandler(t, nil, nil))

	cmd := NewCmdSearch(f)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no JQL and no --mine")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T %v", err, err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("expected VALIDATION_ERROR, got: %s", cliErr.Code)
	}
	if !strings.Contains(cliErr.Message, "Provide a JQL query or use --mine") {
		t.Errorf("unexpected message: %s", cliErr.Message)
	}
}

func TestSearchStatusWithoutMineError(t *testing.T) {
	f, _, _ := newTestSearchFactory(t, searchHandler(t, nil, nil))

	cmd := NewCmdSearch(f)
	cmd.SetArgs([]string{"--status", "Done"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --status without --mine")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T %v", err, err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("expected VALIDATION_ERROR, got: %s", cliErr.Code)
	}
	if !strings.Contains(cliErr.Message, "--status requires --mine or a JQL query") {
		t.Errorf("unexpected message: %s", cliErr.Message)
	}
}

func TestSearchEmptyResults(t *testing.T) {
	f, tio, _ := newTestSearchFactory(t, searchHandler(t, []api.Issue{}, nil))

	opts := &SearchOptions{
		Factory: f,
		Mine:    true,
		Limit:   50,
	}

	err := runSearch(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "No issues found") {
		t.Errorf("expected 'No issues found', got: %s", out)
	}
}

func TestSearchEmptyResultsJSON(t *testing.T) {
	f, tio, _ := newTestSearchFactory(t, searchHandler(t, []api.Issue{}, nil))
	f.OutputJSON = true

	opts := &SearchOptions{
		Factory: f,
		Mine:    true,
		Limit:   50,
	}

	err := runSearch(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
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

func TestSearchJSONOutput(t *testing.T) {
	issues := sampleIssues()
	f, tio, _ := newTestSearchFactory(t, searchHandler(t, issues, nil))
	f.OutputJSON = true

	opts := &SearchOptions{
		Factory: f,
		JQL:     "project = PROJ",
		Limit:   50,
	}

	err := runSearch(opts)
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
}

func TestSearchPagination(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jql") {
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

	f, tio, _ := newTestSearchFactory(t, http.HandlerFunc(handler))
	f.OutputJSON = true

	opts := &SearchOptions{
		Factory: f,
		JQL:     "project = PROJ",
		Limit:   2,
	}

	err := runSearch(opts)
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

func TestSearchQuietMode(t *testing.T) {
	f, tio, _ := newTestSearchFactory(t, searchHandler(t, sampleIssues(), nil))
	f.Quiet = true

	opts := &SearchOptions{
		Factory: f,
		Mine:    true,
		Limit:   50,
	}

	err := runSearch(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tio.OutBuf.String() != "" {
		t.Errorf("expected no output in quiet mode, got: %s", tio.OutBuf.String())
	}
}

func TestSearchFieldsParam(t *testing.T) {
	// Verify that custom --fields are sent as API fields parameter.
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

	f, _, _ := newTestSearchFactory(t, http.HandlerFunc(handler))

	opts := &SearchOptions{
		Factory: f,
		JQL:     "project = PROJ",
		Fields:  []string{"summary", "status"},
		Limit:   50,
	}

	err := runSearch(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(capturedFields) != 2 || capturedFields[0] != "summary" || capturedFields[1] != "status" {
		t.Errorf("expected fields [summary, status], got: %v", capturedFields)
	}
}

func TestSearchDefaultFields(t *testing.T) {
	// Verify default fields are sent when --fields is not set.
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

	f, _, _ := newTestSearchFactory(t, http.HandlerFunc(handler))

	opts := &SearchOptions{
		Factory: f,
		JQL:     "project = PROJ",
		Limit:   50,
	}

	err := runSearch(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should send default fields.
	if len(capturedFields) != 5 {
		t.Errorf("expected 5 default fields, got %d: %v", len(capturedFields), capturedFields)
	}
}

func TestSearchCobraValidation(t *testing.T) {
	// Test that the command validates via Cobra (positional arg parsing).
	f, _, _ := newTestSearchFactory(t, searchHandler(t, sampleIssues(), nil))

	cmd := NewCmdSearch(f)
	cmd.SetArgs([]string{"project = PROJ"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchMineViaCommand(t *testing.T) {
	var capturedJQL string

	f, _, _ := newTestSearchFactory(t, searchHandler(t, sampleIssues(), &capturedJQL))

	cmd := NewCmdSearch(f)
	cmd.SetArgs([]string{"--mine"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "assignee = currentUser() AND resolution = Unresolved"
	if capturedJQL != expected {
		t.Errorf("JQL mismatch:\n  got:  %s\n  want: %s", capturedJQL, expected)
	}
}

func TestSearchMineWithStatusViaCommand(t *testing.T) {
	var capturedJQL string

	f, _, _ := newTestSearchFactory(t, searchHandler(t, sampleIssues(), &capturedJQL))

	cmd := NewCmdSearch(f)
	cmd.SetArgs([]string{"--mine", "--status", "In Progress"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := `assignee = currentUser() AND resolution = Unresolved AND status = "In Progress"`
	if capturedJQL != expected {
		t.Errorf("JQL mismatch:\n  got:  %s\n  want: %s", capturedJQL, expected)
	}
}

func TestSearchTooManyArgs(t *testing.T) {
	f, _, _ := newTestSearchFactory(t, searchHandler(t, nil, nil))

	cmd := NewCmdSearch(f)
	cmd.SetArgs([]string{"query1", "query2"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for too many args")
	}
}

func TestSearchNoPagerFlag(t *testing.T) {
	f, _, _ := newTestSearchFactory(t, searchHandler(t, sampleIssues(), nil))

	cmd := NewCmdSearch(f)
	cmd.SetArgs([]string{"--mine", "--no-pager"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !f.IOStreams.NoPager {
		t.Error("expected NoPager to be set on IOStreams")
	}
}
