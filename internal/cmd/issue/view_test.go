package issue

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/auth"
	"github.com/endgame-build/jira-cli/internal/config"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/iostreams"
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

// newTestViewWebFactory creates a Factory wired with a stored profile (for --web).
// The --web code path needs AuthCredentials() to resolve the instance URL.
func newTestViewWebFactory(t *testing.T) (*factory.Factory, *iostreams.TestIOStreams) {
	t.Helper()
	keyring.MockInit()

	tio := iostreams.Test()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	type profileSetter interface {
		SetProfile(name, instance, user string)
		SetActiveProfile(name string) error
		config.Config
	}
	pc := cfg.(profileSetter)
	pc.SetProfile("default", "mysite.atlassian.net", "user@example.com")
	if err := pc.SetActiveProfile("default"); err != nil {
		t.Fatalf("set active: %v", err)
	}
	if err := pc.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}

	if err := keyring.Set("jira-cli", "default-token", "tok123"); err != nil {
		t.Fatalf("set keyring: %v", err)
	}

	f := factory.NewTestFactory(tio.IOStreams, cfg, nil)
	return f, tio
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

func TestViewDescriptionPlaintext(t *testing.T) {
	// Build an ADF document with structured elements (bullets, code block).
	// ToPlaintext renders these with structure; ExtractText would just concatenate text.
	descJSON := json.RawMessage(`{
		"type": "doc",
		"version": 1,
		"content": [
			{
				"type": "bulletList",
				"content": [
					{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Open the login page"}]}]},
					{"type": "listItem", "content": [{"type": "paragraph", "content": [{"type": "text", "text": "Click submit"}]}]}
				]
			},
			{
				"type": "codeBlock",
				"content": [{"type": "text", "text": "Error: null pointer"}]
			}
		]
	}`)

	issue := api.Issue{
		ID:  "10003",
		Key: "PROJ-789",
		Fields: api.IssueFields{
			Summary:     "Complex description test",
			Description: descJSON,
			Status:      &api.Status{ID: "1", Name: "Open"},
		},
	}

	f, tio, _ := newTestViewFactory(t, issueHandler(issue))
	opts := &ViewOptions{Factory: f, KeyOrID: "PROJ-789"}

	if err := runView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()

	// ToPlaintext renders structured output: bullets with "- " prefix, code block with 4-space indent.
	// ExtractText would just produce "Open the login pageClick submit\nError: null pointer" without structure.
	for _, want := range []string{
		"- Open the login page",
		"- Click submit",
		"    Error: null pointer", // code block indented 4 spaces
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
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

// ──────────────────────────────────────────────
// US-021b: Linked issues, subtasks, comments, --web
// ──────────────────────────────────────────────

func TestViewLinkedIssues(t *testing.T) {
	issue := sampleIssue()
	issue.Fields.IssueLinks = []api.IssueLink{
		{
			ID: "1001",
			Type: &api.IssueLinkType{
				ID:      "10000",
				Name:    "Blocks",
				Inward:  "is blocked by",
				Outward: "blocks",
			},
			OutwardIssue: &api.LinkedIssue{
				ID:  "10002",
				Key: "PROJ-456",
				Fields: &api.LinkedIssueFields{
					Summary: "Blocked task",
					Status:  &api.Status{Name: "In Progress"},
				},
			},
		},
		{
			ID: "1002",
			Type: &api.IssueLinkType{
				ID:      "10001",
				Name:    "Relates",
				Inward:  "relates to",
				Outward: "relates to",
			},
			InwardIssue: &api.LinkedIssue{
				ID:  "10003",
				Key: "PROJ-789",
				Fields: &api.LinkedIssueFields{
					Summary: "Related task",
					Status:  &api.Status{Name: "Done"},
				},
			},
		},
	}

	f, tio, _ := newTestViewFactory(t, issueHandler(issue))
	opts := &ViewOptions{Factory: f, KeyOrID: "PROJ-123"}

	if err := runView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()

	for _, want := range []string{
		"Linked Issues:",
		"blocks PROJ-456 (In Progress)",
		"relates to PROJ-789 (Done)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestViewSubtasks(t *testing.T) {
	issue := sampleIssue()
	issue.Fields.SubTasks = []api.Issue{
		{
			ID:  "10010",
			Key: "PROJ-125",
			Fields: api.IssueFields{
				Summary: "Fix login CSS",
				Status:  &api.Status{Name: "Done"},
			},
		},
		{
			ID:  "10011",
			Key: "PROJ-126",
			Fields: api.IssueFields{
				Summary: "Add error message",
				Status:  &api.Status{Name: "To Do"},
			},
		},
	}

	f, tio, _ := newTestViewFactory(t, issueHandler(issue))
	opts := &ViewOptions{Factory: f, KeyOrID: "PROJ-123"}

	if err := runView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	for _, want := range []string{
		"Subtasks:",
		"PROJ-125  Fix login CSS  [Done]",
		"PROJ-126  Add error message  [To Do]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestViewComments(t *testing.T) {
	issue := sampleIssue()
	issue.Fields.Comment = &api.CommentPage{
		Comments: []api.Comment{
			{
				ID: "100",
				Author: &api.User{
					AccountID:   "abc",
					DisplayName: "Alice",
				},
				Body: json.RawMessage(`{
					"type": "doc", "version": 1,
					"content": [{"type": "paragraph", "content": [{"type": "text", "text": "First comment"}]}]
				}`),
				Created: "2026-02-20T10:00:00.000+0000",
			},
			{
				ID: "101",
				Author: &api.User{
					AccountID:   "def",
					DisplayName: "Bob",
				},
				Body: json.RawMessage(`{
					"type": "doc", "version": 1,
					"content": [{"type": "paragraph", "content": [{"type": "text", "text": "Second comment"}]}]
				}`),
				Created: "2026-02-21T15:00:00.000+0000",
			},
		},
		Total: 2,
	}

	f, tio, _ := newTestViewFactory(t, issueHandler(issue))
	opts := &ViewOptions{Factory: f, KeyOrID: "PROJ-123", Comments: true}

	if err := runView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	for _, want := range []string{
		"Comments (2):",
		"Alice",
		"First comment",
		"Bob",
		"Second comment",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestViewCommentsNotShownWithoutFlag(t *testing.T) {
	issue := sampleIssue()
	issue.Fields.Comment = &api.CommentPage{
		Comments: []api.Comment{
			{
				ID:      "100",
				Author:  &api.User{DisplayName: "Alice"},
				Body:    json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"Secret comment"}]}]}`),
				Created: "2026-02-20T10:00:00.000+0000",
			},
		},
		Total: 1,
	}

	f, tio, _ := newTestViewFactory(t, issueHandler(issue))
	opts := &ViewOptions{Factory: f, KeyOrID: "PROJ-123", Comments: false}

	if err := runView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if strings.Contains(out, "Comments") {
		t.Errorf("output should not contain comments without --comments flag:\n%s", out)
	}
}

func TestViewWeb(t *testing.T) {
	var openedURL string
	mockBrowser := func(url string) error {
		openedURL = url
		return nil
	}

	f, tio := newTestViewWebFactory(t)
	opts := &ViewOptions{
		Factory:     f,
		KeyOrID:     "PROJ-123",
		Web:         true,
		BrowserOpen: mockBrowser,
	}

	if err := runView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify URL was opened.
	want := "https://mysite.atlassian.net/browse/PROJ-123"
	if openedURL != want {
		t.Errorf("browser URL = %q, want %q", openedURL, want)
	}

	// Text mode: should print "Opening ... in browser..."
	out := tio.OutBuf.String()
	if !strings.Contains(out, "Opening") || !strings.Contains(out, "PROJ-123") {
		t.Errorf("output should say opening in browser:\n%s", out)
	}
}

func TestViewWebJSON(t *testing.T) {
	var openedURL string
	mockBrowser := func(url string) error {
		openedURL = url
		return nil
	}

	f, tio := newTestViewWebFactory(t)
	f.OutputJSON = true
	opts := &ViewOptions{
		Factory:     f,
		KeyOrID:     "PROJ-123",
		Web:         true,
		BrowserOpen: mockBrowser,
	}

	if err := runView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Dual action: both print JSON AND open browser.
	want := "https://mysite.atlassian.net/browse/PROJ-123"
	if openedURL != want {
		t.Errorf("browser URL = %q, want %q", openedURL, want)
	}

	// JSON output: {ok:true, url: ...}
	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}
	if result["ok"] != true {
		t.Errorf("ok = %v, want true", result["ok"])
	}
	if result["url"] != want {
		t.Errorf("url = %v, want %q", result["url"], want)
	}
}

func TestViewCommentsWithFieldsFilter(t *testing.T) {
	// Verify that --comments works even when --fields is specified without "comments".
	issue := sampleIssue()
	issue.Fields.Comment = &api.CommentPage{
		Comments: []api.Comment{
			{
				ID:      "100",
				Author:  &api.User{DisplayName: "Alice"},
				Body:    json.RawMessage(`{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"Visible comment"}]}]}`),
				Created: "2026-02-20T10:00:00.000+0000",
			},
		},
		Total: 1,
	}

	f, tio, _ := newTestViewFactory(t, issueHandler(issue))
	opts := &ViewOptions{
		Factory:  f,
		KeyOrID:  "PROJ-123",
		Comments: true,
		Fields:   []string{"key", "summary"}, // does NOT include "comments"
	}

	if err := runView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()

	// --comments is a standalone flag; should not be gated by --fields.
	if !strings.Contains(out, "Visible comment") {
		t.Errorf("--comments should show comments regardless of --fields filter:\n%s", out)
	}
	if !strings.Contains(out, "Comments (1):") {
		t.Errorf("output should contain comment section header:\n%s", out)
	}

	// --fields=key,summary should still filter other fields.
	if strings.Contains(out, "Jane Doe") {
		t.Errorf("assignee should be filtered out by --fields:\n%s", out)
	}
}

func TestViewWebQuiet(t *testing.T) {
	var openedURL string
	mockBrowser := func(url string) error {
		openedURL = url
		return nil
	}

	f, tio := newTestViewWebFactory(t)
	f.Quiet = true
	opts := &ViewOptions{
		Factory:     f,
		KeyOrID:     "PROJ-123",
		Web:         true,
		BrowserOpen: mockBrowser,
	}

	if err := runView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Browser should still open.
	if openedURL == "" {
		t.Error("browser should have been opened in quiet mode")
	}

	// Stdout should be empty.
	out := tio.OutBuf.String()
	if out != "" {
		t.Errorf("output should be empty in quiet mode, got:\n%s", out)
	}
}
