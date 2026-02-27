package project

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

func sampleProjectDetail() map[string]interface{} {
	return map[string]interface{}{
		"id":             "10001",
		"key":            "PROJ",
		"name":           "My Project",
		"description":    "A sample project for testing",
		"lead":           map[string]interface{}{"accountId": "abc123", "displayName": "Alice"},
		"projectTypeKey": "software",
		"issueTypes": []map[string]interface{}{
			{"id": "10001", "name": "Bug", "subtask": false},
			{"id": "10002", "name": "Story", "subtask": false},
			{"id": "10003", "name": "Sub-task", "subtask": true},
		},
		"url":        "https://test.atlassian.net/projects/PROJ",
		"simplified": false,
		"style":      "classic",
	}
}

func projectViewHandler(project map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/project/") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(project)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestProjectViewFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams) {
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
	return f, tio
}

func TestProjectView_Success(t *testing.T) {
	project := sampleProjectDetail()
	f, tio := newTestProjectViewFactory(t, projectViewHandler(project))

	opts := &ProjectViewOptions{
		Factory: f,
		KeyOrID: "PROJ",
	}

	if err := runProjectView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()

	for _, want := range []string{
		"PROJ",
		"My Project",
		"Alice",
		"A sample project for testing",
		"software",
		"Bug, Story, Sub-task",
		"https://test.atlassian.net/projects/PROJ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestProjectView_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["No project could be found with key 'XYZ'."]}`))
	})

	f, _ := newTestProjectViewFactory(t, handler)

	opts := &ProjectViewOptions{
		Factory: f,
		KeyOrID: "XYZ",
	}

	err := runProjectView(opts)
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

func TestProjectView_JSON(t *testing.T) {
	project := sampleProjectDetail()
	f, tio := newTestProjectViewFactory(t, projectViewHandler(project))
	f.OutputJSON = true

	opts := &ProjectViewOptions{
		Factory: f,
		KeyOrID: "PROJ",
	}

	if err := runProjectView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	// Should be a bare project object (not wrapped in envelope).
	if result["key"] != "PROJ" {
		t.Errorf("key = %v, want PROJ", result["key"])
	}
	if result["name"] != "My Project" {
		t.Errorf("name = %v, want My Project", result["name"])
	}
	if result["projectTypeKey"] != "software" {
		t.Errorf("projectTypeKey = %v, want software", result["projectTypeKey"])
	}

	// Verify issue types are included.
	issueTypes, ok := result["issueTypes"].([]interface{})
	if !ok {
		t.Fatal("issueTypes should be an array")
	}
	if len(issueTypes) != 3 {
		t.Errorf("issueTypes length = %d, want 3", len(issueTypes))
	}
}

func TestProjectView_InvalidKey(t *testing.T) {
	f, _ := newTestProjectViewFactory(t, projectViewHandler(sampleProjectDetail()))

	cmd := NewCmdView(f)
	cmd.SetArgs([]string{"bad-key-123"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "Invalid project key") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestProjectView_IssueTypesDisplay(t *testing.T) {
	project := sampleProjectDetail()
	f, tio := newTestProjectViewFactory(t, projectViewHandler(project))

	opts := &ProjectViewOptions{
		Factory: f,
		KeyOrID: "PROJ",
	}

	if err := runProjectView(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()

	// Issue types should be comma-separated.
	if !strings.Contains(out, "Bug, Story, Sub-task") {
		t.Errorf("issue types should be comma-separated, got:\n%s", out)
	}
}
