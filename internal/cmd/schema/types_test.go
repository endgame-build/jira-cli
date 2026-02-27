package schema

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

func newTestSchemaTypesFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams) {
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

func sampleIssueTypes() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":             "10001",
			"name":           "Bug",
			"description":    "A problem which impairs or prevents the functions of the product.",
			"subtask":        false,
			"iconUrl":        "https://example.com/bug.svg",
			"hierarchyLevel": 0,
		},
		{
			"id":             "10002",
			"name":           "Story",
			"description":    "A user story",
			"subtask":        false,
			"iconUrl":        "https://example.com/story.svg",
			"hierarchyLevel": 0,
		},
		{
			"id":             "10003",
			"name":           "Sub-task",
			"description":    "The sub-task of the issue",
			"subtask":        true,
			"iconUrl":        "https://example.com/subtask.svg",
			"hierarchyLevel": -1,
		},
	}
}

func typesHandler(t *testing.T, globalTypes, projectTypes []map[string]interface{}, projectDetail map[string]interface{}) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET /issuetype/project?projectId=... — project-scoped types.
		if r.Method == http.MethodGet && strings.Contains(path, "/issuetype/project") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(projectTypes)
			return
		}

		// GET /issuetype — global types.
		if r.Method == http.MethodGet && strings.HasSuffix(path, "/issuetype") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(globalTypes)
			return
		}

		// GET /project/{keyOrId} — project detail.
		if r.Method == http.MethodGet && strings.Contains(path, "/project/") {
			if projectDetail == nil {
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"No project could be found"},
				})
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(projectDetail)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func TestSchemaTypes_Global(t *testing.T) {
	handler := typesHandler(t, sampleIssueTypes(), nil, nil)
	f, tio := newTestSchemaTypesFactory(t, handler)

	cmd := NewCmdTypes(f)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Bug") {
		t.Errorf("output should contain 'Bug', got: %s", out)
	}
	if !strings.Contains(out, "Story") {
		t.Errorf("output should contain 'Story', got: %s", out)
	}
	if !strings.Contains(out, "Sub-task") {
		t.Errorf("output should contain 'Sub-task', got: %s", out)
	}
	if !strings.Contains(out, "yes") {
		t.Errorf("output should contain 'yes' for subtask, got: %s", out)
	}
}

func TestSchemaTypes_ProjectScoped(t *testing.T) {
	projectTypes := []map[string]interface{}{
		{
			"id":             "10001",
			"name":           "Bug",
			"description":    "A bug in the project",
			"subtask":        false,
			"iconUrl":        "https://example.com/bug.svg",
			"hierarchyLevel": 0,
		},
	}
	projectDetail := map[string]interface{}{
		"id":   "10100",
		"key":  "PROJ",
		"name": "My Project",
	}

	handler := typesHandler(t, sampleIssueTypes(), projectTypes, projectDetail)
	f, tio := newTestSchemaTypesFactory(t, handler)

	cmd := NewCmdTypes(f)
	cmd.SetArgs([]string{"--project", "PROJ"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Bug") {
		t.Errorf("output should contain 'Bug', got: %s", out)
	}
	// Should only have project-scoped types (1 type), not all 3 global types.
	if strings.Contains(out, "Story") {
		t.Errorf("output should NOT contain 'Story' (project-scoped), got: %s", out)
	}
}

func TestSchemaTypes_ProjectNotFound(t *testing.T) {
	handler := typesHandler(t, sampleIssueTypes(), nil, nil)
	f, _ := newTestSchemaTypesFactory(t, handler)

	cmd := NewCmdTypes(f)
	cmd.SetArgs([]string{"--project", "NOPE"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for project 404")
	}
	errMsg := strings.ToLower(err.Error())
	if !strings.Contains(errMsg, "not_found") && !strings.Contains(errMsg, "not found") {
		t.Errorf("expected NOT_FOUND error, got: %v", err)
	}
}

func TestSchemaTypes_JSON(t *testing.T) {
	handler := typesHandler(t, sampleIssueTypes(), nil, nil)
	f, tio := newTestSchemaTypesFactory(t, handler)
	f.OutputJSON = true

	cmd := NewCmdTypes(f)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var envelope struct {
		Data       []json.RawMessage `json:"data"`
		Pagination *json.RawMessage  `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON: %v\nOutput: %s", err, out)
	}
	if len(envelope.Data) != 3 {
		t.Errorf("data length = %d, want 3", len(envelope.Data))
	}
	if envelope.Pagination != nil {
		t.Error("pagination should be null for unpaginated list")
	}

	// Verify structure includes expected JSON fields.
	var firstType struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Description    string `json:"description"`
		Subtask        bool   `json:"subtask"`
		IconURL        string `json:"iconUrl"`
		HierarchyLevel int    `json:"hierarchyLevel"`
	}
	if err := json.Unmarshal(envelope.Data[0], &firstType); err != nil {
		t.Fatalf("failed to parse first type: %v", err)
	}
	if firstType.ID != "10001" {
		t.Errorf("first type id = %q, want '10001'", firstType.ID)
	}
	if firstType.Name != "Bug" {
		t.Errorf("first type name = %q, want 'Bug'", firstType.Name)
	}
	if firstType.Subtask {
		t.Error("first type subtask should be false")
	}
	if firstType.IconURL != "https://example.com/bug.svg" {
		t.Errorf("first type iconUrl = %q, want 'https://example.com/bug.svg'", firstType.IconURL)
	}
}
