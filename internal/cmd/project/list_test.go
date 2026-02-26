package project

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/auth"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/iostreams"
)

func projectListHandler(projects []map[string]interface{}, total int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// GET /project/search
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/project/search") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values":     projects,
				"maxResults": 50,
				"total":      total,
				"startAt":    0,
				"isLast":     len(projects) >= total,
			})
			return
		}

		// GET /myself — used by CheckEmptyResultsAuth.
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/myself") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"accountId":   "abc123",
				"displayName": "Test User",
			})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func newTestProjectListFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams) {
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

func sampleProjects() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":             "10001",
			"key":            "PROJ",
			"name":           "My Project",
			"description":    "A sample project",
			"lead":           map[string]interface{}{"accountId": "abc123", "displayName": "Alice"},
			"projectTypeKey": "software",
			"issueTypes":     []map[string]interface{}{},
			"url":            "https://test.atlassian.net/projects/PROJ",
		},
		{
			"id":             "10002",
			"key":            "OPS",
			"name":           "Operations",
			"description":    "Ops project",
			"lead":           map[string]interface{}{"accountId": "def456", "displayName": "Bob"},
			"projectTypeKey": "service_desk",
			"issueTypes":     []map[string]interface{}{},
			"url":            "https://test.atlassian.net/projects/OPS",
		},
	}
}

func TestProjectList_Success(t *testing.T) {
	projects := sampleProjects()
	f, tio := newTestProjectListFactory(t, projectListHandler(projects, 2))

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "PROJ") {
		t.Errorf("output should contain project key PROJ, got: %s", out)
	}
	if !strings.Contains(out, "My Project") {
		t.Errorf("output should contain project name, got: %s", out)
	}
	if !strings.Contains(out, "Alice") {
		t.Errorf("output should contain lead name Alice, got: %s", out)
	}
	if !strings.Contains(out, "software") {
		t.Errorf("output should contain project type, got: %s", out)
	}
	if !strings.Contains(out, "OPS") {
		t.Errorf("output should contain project key OPS, got: %s", out)
	}
	if !strings.Contains(out, "Bob") {
		t.Errorf("output should contain lead name Bob, got: %s", out)
	}
}

func TestProjectList_Pagination(t *testing.T) {
	projects := sampleProjects()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/project/search") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values":     projects,
				"maxResults": 2,
				"total":      10,
				"startAt":    0,
				"isLast":     false,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	f, tio := newTestProjectListFactory(t, handler)
	f.OutputJSON = true

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{"--limit", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var envelope struct {
		Pagination struct {
			Total       *int `json:"total"`
			HasNextPage bool `json:"has_next_page"`
			Limit       int  `json:"limit"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if !envelope.Pagination.HasNextPage {
		t.Error("has_next_page should be true when total > offset + count")
	}
	if envelope.Pagination.Limit != 2 {
		t.Errorf("limit = %d, want 2", envelope.Pagination.Limit)
	}
	if envelope.Pagination.Total == nil || *envelope.Pagination.Total != 10 {
		t.Errorf("total should be 10")
	}
}

func TestProjectList_Empty_Text(t *testing.T) {
	f, tio := newTestProjectListFactory(t, projectListHandler(nil, 0))

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "No projects found") {
		t.Errorf("expected empty message, got: %s", out)
	}
}

func TestProjectList_Empty_JSON(t *testing.T) {
	f, tio := newTestProjectListFactory(t, projectListHandler(nil, 0))
	f.OutputJSON = true

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var envelope struct {
		Data       []json.RawMessage `json:"data"`
		Pagination struct {
			Total *int `json:"total"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON: %v\nOutput: %s", err, out)
	}
	if len(envelope.Data) != 0 {
		t.Errorf("data length = %d, want 0", len(envelope.Data))
	}
	if envelope.Pagination.Total == nil || *envelope.Pagination.Total != 0 {
		t.Errorf("pagination.total should be 0 for empty results")
	}
}

func TestProjectList_EmptyWithAuthCheck(t *testing.T) {
	// Simulate unauthenticated: /project/search returns empty, /myself returns 401.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/project/search") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values":     []interface{}{},
				"maxResults": 50,
				"total":      0,
				"startAt":    0,
				"isLast":     true,
			})
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/myself") {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"message": "Client must be authenticated to access this resource.",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	f, _ := newTestProjectListFactory(t, handler)

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected auth error for unauthenticated empty results, got nil")
	}
	errStr := strings.ToLower(err.Error())
	if !strings.Contains(errStr, "auth") && !strings.Contains(errStr, "401") {
		t.Errorf("expected auth error, got: %v", err)
	}
}

func TestProjectList_JSON(t *testing.T) {
	projects := sampleProjects()
	f, tio := newTestProjectListFactory(t, projectListHandler(projects, 2))
	f.OutputJSON = true

	cmd := NewCmdList(f)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var envelope struct {
		Data       []json.RawMessage `json:"data"`
		Pagination struct {
			Offset      int  `json:"offset"`
			Limit       int  `json:"limit"`
			Total       *int `json:"total"`
			HasNextPage bool `json:"has_next_page"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
	}
	if len(envelope.Data) != 2 {
		t.Errorf("data length = %d, want 2", len(envelope.Data))
	}
	if envelope.Pagination.Total == nil || *envelope.Pagination.Total != 2 {
		t.Errorf("pagination.total should be 2")
	}
	if envelope.Pagination.HasNextPage {
		t.Error("has_next_page should be false when all results returned")
	}

	// Verify project fields in JSON.
	var first struct {
		Key            string `json:"key"`
		Name           string `json:"name"`
		ProjectTypeKey string `json:"projectTypeKey"`
	}
	if err := json.Unmarshal(envelope.Data[0], &first); err != nil {
		t.Fatalf("failed to parse first project: %v", err)
	}
	if first.Key != "PROJ" {
		t.Errorf("first project key = %q, want PROJ", first.Key)
	}
	if first.Name != "My Project" {
		t.Errorf("first project name = %q, want My Project", first.Name)
	}
	if first.ProjectTypeKey != "software" {
		t.Errorf("first project type = %q, want software", first.ProjectTypeKey)
	}
}
