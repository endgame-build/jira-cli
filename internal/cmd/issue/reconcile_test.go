package issue

import (
	"encoding/json"
	"errors"
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

// reconcileHandler serves search, transitions, and delete endpoints for reconcile tests.
func reconcileHandler(issues []api.Issue, transitions []api.Transition) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// POST /search/jql
		if r.Method == http.MethodPost && strings.HasSuffix(path, "/search/jql") {
			json.NewEncoder(w).Encode(api.SearchResults{
				Issues: issues,
				IsLast: true,
			})
			return
		}

		// GET /issue/{key}/transitions
		if r.Method == http.MethodGet && strings.HasSuffix(path, "/transitions") {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"transitions": transitions,
			})
			return
		}

		// POST /issue/{key}/transitions (DoTransition)
		if r.Method == http.MethodPost && strings.HasSuffix(path, "/transitions") {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// DELETE /issue/{key}
		if r.Method == http.MethodDelete && strings.Contains(path, "/issue/") {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestReconcileFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams, *httptest.Server) {
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

// writeMarkdownFile creates a minimal markdown file with the given key.
func writeMarkdownFile(t *testing.T, dir, key, summary string) {
	t.Helper()
	content := "---\nkey: " + key + "\nsummary: " + summary + "\n---\n\nDescription.\n"
	path := filepath.Join(dir, key+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileListOrphans(t *testing.T) {
	dir := t.TempDir()
	writeMarkdownFile(t, dir, "PROJ-1", "Exists locally")

	jiraIssues := []api.Issue{
		{Key: "PROJ-1", Fields: api.IssueFields{Summary: "Exists locally", Status: &api.Status{Name: "To Do"}}},
		{Key: "PROJ-2", Fields: api.IssueFields{Summary: "Orphan one", Status: &api.Status{Name: "In Progress"}}},
		{Key: "PROJ-3", Fields: api.IssueFields{Summary: "Orphan two", Status: &api.Status{Name: "Done"}}},
	}

	f, tio, _ := newTestReconcileFactory(t, reconcileHandler(jiraIssues, nil))

	opts := &ReconcileOptions{
		Factory: f,
		Dir:     dir,
		Project: "PROJ",
		Action:  "list",
	}

	if err := runReconcile(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ-2") {
		t.Errorf("expected PROJ-2 in output, got: %s", out)
	}
	if !strings.Contains(out, "PROJ-3") {
		t.Errorf("expected PROJ-3 in output, got: %s", out)
	}
	if strings.Contains(out, "PROJ-1") {
		t.Errorf("PROJ-1 should not appear as orphan, got: %s", out)
	}
}

func TestReconcileNoOrphans(t *testing.T) {
	dir := t.TempDir()
	writeMarkdownFile(t, dir, "PROJ-1", "Exists")

	jiraIssues := []api.Issue{
		{Key: "PROJ-1", Fields: api.IssueFields{Summary: "Exists", Status: &api.Status{Name: "To Do"}}},
	}

	f, tio, _ := newTestReconcileFactory(t, reconcileHandler(jiraIssues, nil))

	opts := &ReconcileOptions{
		Factory: f,
		Dir:     dir,
		Project: "PROJ",
		Action:  "list",
	}

	if err := runReconcile(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "No orphaned issues") {
		t.Errorf("expected no-orphans message, got: %s", out)
	}
}

func TestReconcileExcludesEpicKey(t *testing.T) {
	dir := t.TempDir()
	writeMarkdownFile(t, dir, "PROJ-10", "Epic file")

	jiraIssues := []api.Issue{
		{Key: "PROJ-10", Fields: api.IssueFields{Summary: "The Epic", Status: &api.Status{Name: "To Do"}}},
		{Key: "PROJ-11", Fields: api.IssueFields{Summary: "Child story", Status: &api.Status{Name: "To Do"}}},
	}

	f, tio, _ := newTestReconcileFactory(t, reconcileHandler(jiraIssues, nil))

	opts := &ReconcileOptions{
		Factory: f,
		Dir:     dir,
		Epic:    "PROJ-10",
		Action:  "list",
	}

	if err := runReconcile(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ-11") {
		t.Errorf("expected PROJ-11 as orphan, got: %s", out)
	}
	// PROJ-10 is the epic itself — should not appear.
}

func TestReconcileExcludesTempKeys(t *testing.T) {
	dir := t.TempDir()
	writeMarkdownFile(t, dir, "PROJ-NEW-1", "New issue")
	writeMarkdownFile(t, dir, "PROJ-1", "Existing")

	jiraIssues := []api.Issue{
		{Key: "PROJ-1", Fields: api.IssueFields{Summary: "Existing", Status: &api.Status{Name: "To Do"}}},
		{Key: "PROJ-2", Fields: api.IssueFields{Summary: "Orphan", Status: &api.Status{Name: "To Do"}}},
	}

	f, tio, _ := newTestReconcileFactory(t, reconcileHandler(jiraIssues, nil))

	opts := &ReconcileOptions{
		Factory: f,
		Dir:     dir,
		Project: "PROJ",
		Action:  "list",
	}

	if err := runReconcile(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ-2") {
		t.Errorf("expected PROJ-2 as orphan, got: %s", out)
	}
}

func TestReconcileCloseOrphans(t *testing.T) {
	dir := t.TempDir()
	writeMarkdownFile(t, dir, "PROJ-1", "Exists")

	jiraIssues := []api.Issue{
		{Key: "PROJ-1", Fields: api.IssueFields{Summary: "Exists", Status: &api.Status{Name: "To Do"}}},
		{Key: "PROJ-2", Fields: api.IssueFields{Summary: "Orphan", Status: &api.Status{Name: "In Progress"}}},
	}
	transitions := []api.Transition{
		{ID: "31", Name: "Done", To: &api.Status{Name: "Done"}},
	}

	var transitionCalled bool
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/transitions") {
			transitionCalled = true
		}
		reconcileHandler(jiraIssues, transitions)(w, r)
	}

	f, tio, _ := newTestReconcileFactory(t, http.HandlerFunc(handler))

	opts := &ReconcileOptions{
		Factory:      f,
		Dir:          dir,
		Project:      "PROJ",
		Action:       "close",
		TargetStatus: "Done",
	}

	if err := runReconcile(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !transitionCalled {
		t.Error("expected transition API call")
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "closed") {
		t.Errorf("expected 'closed' in output, got: %s", out)
	}
}

func TestReconcileDeleteOrphans(t *testing.T) {
	dir := t.TempDir()
	writeMarkdownFile(t, dir, "PROJ-1", "Exists")

	jiraIssues := []api.Issue{
		{Key: "PROJ-1", Fields: api.IssueFields{Summary: "Exists", Status: &api.Status{Name: "To Do"}}},
		{Key: "PROJ-2", Fields: api.IssueFields{Summary: "Orphan", Status: &api.Status{Name: "To Do"}}},
	}

	var deleteCalled bool
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleteCalled = true
		}
		reconcileHandler(jiraIssues, nil)(w, r)
	}

	f, tio, _ := newTestReconcileFactory(t, http.HandlerFunc(handler))

	opts := &ReconcileOptions{
		Factory: f,
		Dir:     dir,
		Project: "PROJ",
		Action:  "delete",
		Yes:     true,
	}

	if err := runReconcile(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !deleteCalled {
		t.Error("expected DELETE API call")
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "deleted") {
		t.Errorf("expected 'deleted' in output, got: %s", out)
	}
}

func TestReconcileDryRun(t *testing.T) {
	dir := t.TempDir()
	writeMarkdownFile(t, dir, "PROJ-1", "Exists")

	jiraIssues := []api.Issue{
		{Key: "PROJ-1", Fields: api.IssueFields{Summary: "Exists", Status: &api.Status{Name: "To Do"}}},
		{Key: "PROJ-2", Fields: api.IssueFields{Summary: "Orphan", Status: &api.Status{Name: "To Do"}}},
	}

	var mutationCalled bool
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete || (r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/transitions")) {
			mutationCalled = true
		}
		reconcileHandler(jiraIssues, nil)(w, r)
	}

	f, tio, _ := newTestReconcileFactory(t, http.HandlerFunc(handler))
	f.DryRun = true

	opts := &ReconcileOptions{
		Factory: f,
		Dir:     dir,
		Project: "PROJ",
		Action:  "delete",
	}

	if err := runReconcile(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mutationCalled {
		t.Error("expected no mutation calls during dry-run")
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "DRY RUN") {
		t.Errorf("expected DRY RUN in output, got: %s", out)
	}
	if !strings.Contains(out, "PROJ-2") {
		t.Errorf("expected PROJ-2 in dry-run output, got: %s", out)
	}
}

func TestReconcileDryRunJSON(t *testing.T) {
	dir := t.TempDir()
	writeMarkdownFile(t, dir, "PROJ-1", "Exists")

	jiraIssues := []api.Issue{
		{Key: "PROJ-1", Fields: api.IssueFields{Summary: "Exists", Status: &api.Status{Name: "To Do"}}},
		{Key: "PROJ-2", Fields: api.IssueFields{Summary: "Orphan", Status: &api.Status{Name: "To Do"}}},
	}

	f, tio, _ := newTestReconcileFactory(t, reconcileHandler(jiraIssues, nil))
	f.DryRun = true
	f.OutputJSON = true

	opts := &ReconcileOptions{
		Factory: f,
		Dir:     dir,
		Project: "PROJ",
		Action:  "list",
	}

	if err := runReconcile(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}

	if result["dry_run"] != true {
		t.Errorf("expected dry_run:true, got: %v", result["dry_run"])
	}
	payload := result["payload"].(map[string]interface{})
	if int(payload["count"].(float64)) != 1 {
		t.Errorf("expected count:1, got: %v", payload["count"])
	}
}

func TestReconcileJSON(t *testing.T) {
	dir := t.TempDir()
	writeMarkdownFile(t, dir, "PROJ-1", "Exists")

	jiraIssues := []api.Issue{
		{Key: "PROJ-1", Fields: api.IssueFields{Summary: "Exists", Status: &api.Status{Name: "To Do"}}},
		{Key: "PROJ-2", Fields: api.IssueFields{Summary: "Orphan", Status: &api.Status{Name: "To Do"}}},
	}

	f, tio, _ := newTestReconcileFactory(t, reconcileHandler(jiraIssues, nil))
	f.OutputJSON = true

	opts := &ReconcileOptions{
		Factory: f,
		Dir:     dir,
		Project: "PROJ",
		Action:  "list",
	}

	if err := runReconcile(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}

	if result["ok"] != true {
		t.Errorf("expected ok:true, got: %v", result["ok"])
	}
	if int(result["count"].(float64)) != 1 {
		t.Errorf("expected count:1, got: %v", result["count"])
	}
}

func TestReconcileQuiet(t *testing.T) {
	dir := t.TempDir()
	writeMarkdownFile(t, dir, "PROJ-1", "Exists")

	jiraIssues := []api.Issue{
		{Key: "PROJ-2", Fields: api.IssueFields{Summary: "Orphan", Status: &api.Status{Name: "To Do"}}},
	}

	f, tio, _ := newTestReconcileFactory(t, reconcileHandler(jiraIssues, nil))
	f.Quiet = true

	opts := &ReconcileOptions{
		Factory: f,
		Dir:     dir,
		Project: "PROJ",
		Action:  "list",
	}

	if err := runReconcile(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tio.OutBuf.Len() > 0 {
		t.Errorf("expected no output in quiet mode, got: %s", tio.OutBuf.String())
	}
}

func TestReconcileJQL(t *testing.T) {
	dir := t.TempDir()
	writeMarkdownFile(t, dir, "PROJ-20", "Epic one")

	jiraIssues := []api.Issue{
		{Key: "PROJ-20", Fields: api.IssueFields{Summary: "Epic one", Status: &api.Status{Name: "To Do"}}},
		{Key: "PROJ-21", Fields: api.IssueFields{Summary: "Child", Status: &api.Status{Name: "To Do"}}},
		{Key: "PROJ-48", Fields: api.IssueFields{Summary: "Epic two", Status: &api.Status{Name: "To Do"}}},
	}

	var capturedJQL string
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jql") {
			var body map[string]interface{}
			json.NewDecoder(r.Body).Decode(&body)
			if jql, ok := body["jql"].(string); ok {
				capturedJQL = jql
			}
		}
		reconcileHandler(jiraIssues, nil)(w, r)
	}

	f, tio, _ := newTestReconcileFactory(t, http.HandlerFunc(handler))

	opts := &ReconcileOptions{
		Factory: f,
		Dir:     dir,
		JQL:     "parent in (PROJ-20, PROJ-48) OR key in (PROJ-20, PROJ-48)",
		Action:  "list",
	}

	if err := runReconcile(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedJQL != "parent in (PROJ-20, PROJ-48) OR key in (PROJ-20, PROJ-48)" {
		t.Errorf("expected raw JQL passthrough, got: %s", capturedJQL)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ-21") {
		t.Errorf("expected PROJ-21 as orphan, got: %s", out)
	}
	if !strings.Contains(out, "PROJ-48") {
		t.Errorf("expected PROJ-48 as orphan, got: %s", out)
	}
}

func TestReconcileValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{
			name:    "missing --dir",
			args:    []string{"--project", "PROJ"},
			wantMsg: "--dir",
		},
		{
			name:    "missing scope",
			args:    []string{"--dir", "/tmp"},
			wantMsg: "--epic, --project, or --jql",
		},
		{
			name:    "both scope flags",
			args:    []string{"--dir", "/tmp", "--epic", "PROJ-1", "--project", "PROJ"},
			wantMsg: "--epic and --project are mutually exclusive",
		},
		{
			name:    "jql and epic conflict",
			args:    []string{"--dir", "/tmp", "--jql", "project = X", "--epic", "PROJ-1"},
			wantMsg: "--jql is mutually exclusive with --epic and --project",
		},
		{
			name:    "jql and project conflict",
			args:    []string{"--dir", "/tmp", "--jql", "project = X", "--project", "PROJ"},
			wantMsg: "--jql is mutually exclusive with --epic and --project",
		},
		{
			name:    "invalid action",
			args:    []string{"--dir", "/tmp", "--project", "PROJ", "--action", "nope"},
			wantMsg: "invalid --action",
		},
		{
			name:    "delete without yes",
			args:    []string{"--dir", "/tmp", "--project", "PROJ", "--action", "delete"},
			wantMsg: "--yes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := factory.NewTestFactory(iostreams.Test().IOStreams, nil, nil)
			cmd := NewCmdReconcile(f)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}

			var cliErr *clierrors.CLIError
			if !errors.As(err, &cliErr) {
				t.Fatalf("expected CLIError, got: %T: %v", err, err)
			}
			if !strings.Contains(cliErr.Message, tt.wantMsg) {
				t.Errorf("expected message containing %q, got: %s", tt.wantMsg, cliErr.Message)
			}
		})
	}
}

func TestReconcileCloseSkipsUnmatchedTransition(t *testing.T) {
	dir := t.TempDir()
	writeMarkdownFile(t, dir, "PROJ-1", "Exists")

	jiraIssues := []api.Issue{
		{Key: "PROJ-1", Fields: api.IssueFields{Summary: "Exists", Status: &api.Status{Name: "To Do"}}},
		{Key: "PROJ-2", Fields: api.IssueFields{Summary: "Orphan", Status: &api.Status{Name: "Done"}}},
	}
	// No transition matches "Done" (already there).
	transitions := []api.Transition{
		{ID: "21", Name: "In Progress", To: &api.Status{Name: "In Progress"}},
	}

	f, tio, _ := newTestReconcileFactory(t, reconcileHandler(jiraIssues, transitions))

	opts := &ReconcileOptions{
		Factory:      f,
		Dir:          dir,
		Project:      "PROJ",
		Action:       "close",
		TargetStatus: "Done",
	}

	if err := runReconcile(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	errOut := tio.ErrBuf.String()
	if !strings.Contains(errOut, "warning") {
		t.Errorf("expected warning about skipped transition, got stderr: %s", errOut)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "skipped") {
		t.Errorf("expected 'skipped' in output, got: %s", out)
	}
}
