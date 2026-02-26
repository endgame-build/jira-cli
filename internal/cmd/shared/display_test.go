package shared

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/auth"
)

func sampleIssues() []api.Issue {
	return []api.Issue{
		{
			ID:   "10001",
			Key:  "PROJ-1",
			Self: "https://test.atlassian.net/rest/api/3/issue/10001",
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
				Labels:    []string{"frontend"},
			},
		},
	}
}

func TestFilterIssueFieldsNilPassthrough(t *testing.T) {
	issues := sampleIssues()
	result := FilterIssueFields(issues, nil)

	// nil wantFields returns original slice unmodified.
	returned, ok := result.([]api.Issue)
	if !ok {
		t.Fatalf("expected []api.Issue, got %T", result)
	}
	if len(returned) != 1 || returned[0].Key != "PROJ-1" {
		t.Errorf("expected original issues, got: %v", returned)
	}
}

func TestFilterIssueFieldsSingleField(t *testing.T) {
	issues := sampleIssues()
	wantFields := map[string]bool{"summary": true}
	result := FilterIssueFields(issues, wantFields)

	b, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(b, &items); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}

	item := items[0]
	// Always-included fields.
	if item["id"] != "10001" {
		t.Errorf("id = %v, want 10001", item["id"])
	}
	if item["key"] != "PROJ-1" {
		t.Errorf("key = %v, want PROJ-1", item["key"])
	}
	if item["self"] == nil {
		t.Error("self should be present")
	}

	fields := item["fields"].(map[string]interface{})
	if fields["summary"] != "Fix login bug" {
		t.Errorf("summary = %v, want 'Fix login bug'", fields["summary"])
	}
	// status should not be present since it wasn't requested.
	if _, exists := fields["status"]; exists {
		t.Error("status should not be present when not requested")
	}
	if _, exists := fields["assignee"]; exists {
		t.Error("assignee should not be present when not requested")
	}
}

func TestFilterIssueFieldsTypeAlias(t *testing.T) {
	issues := sampleIssues()
	// "type" should map to "issuetype" in the output.
	wantFields := map[string]bool{"type": true}
	result := FilterIssueFields(issues, wantFields)

	b, _ := json.Marshal(result)
	var items []map[string]interface{}
	json.Unmarshal(b, &items)

	fields := items[0]["fields"].(map[string]interface{})
	if fields["issuetype"] == nil {
		t.Error("'type' alias should produce 'issuetype' in output")
	}
	if _, exists := fields["summary"]; exists {
		t.Error("summary should not be present when only type was requested")
	}
}

func TestFilterIssueFieldsAllFields(t *testing.T) {
	issues := sampleIssues()
	wantFields := map[string]bool{
		"summary": true, "status": true, "issuetype": true,
		"priority": true, "assignee": true, "labels": true,
	}
	result := FilterIssueFields(issues, wantFields)

	b, _ := json.Marshal(result)
	var items []map[string]interface{}
	json.Unmarshal(b, &items)

	fields := items[0]["fields"].(map[string]interface{})
	for _, name := range []string{"summary", "status", "issuetype", "priority", "assignee", "labels"} {
		if fields[name] == nil {
			t.Errorf("field %q should be present", name)
		}
	}
}

func TestFilterIssueFieldsEmptyMap(t *testing.T) {
	issues := sampleIssues()
	// Empty map (not nil) — no fields requested, only id/key/self.
	wantFields := map[string]bool{}
	result := FilterIssueFields(issues, wantFields)

	b, _ := json.Marshal(result)
	var items []map[string]interface{}
	json.Unmarshal(b, &items)

	fields := items[0]["fields"].(map[string]interface{})
	if len(fields) != 0 {
		t.Errorf("expected empty fields map, got %d keys: %v", len(fields), fields)
	}
	// id/key/self still present at top level.
	if items[0]["key"] != "PROJ-1" {
		t.Error("key should always be present")
	}
}

// --- CheckEmptyResultsAuth tests ---

func newTestClient(t *testing.T, handler http.Handler) *api.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	creds := &auth.Credentials{
		Instance: "test.atlassian.net",
		User:     "test@example.com",
		Token:    "test-token",
	}
	return api.NewClient(creds, api.WithBaseURL(srv.URL))
}

func TestCheckEmptyResultsAuthValidCredentials(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.User{AccountID: "123", DisplayName: "Test"})
	}))

	var stderr bytes.Buffer
	err := CheckEmptyResultsAuth(context.Background(), client, &stderr)
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
	if stderr.Len() > 0 {
		t.Errorf("expected no stderr output, got: %s", stderr.String())
	}
}

func TestCheckEmptyResultsAuthBadCredentials(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errorMessages":["Client must be authenticated"]}`))
	}))

	var stderr bytes.Buffer
	err := CheckEmptyResultsAuth(context.Background(), client, &stderr)
	if err == nil {
		t.Fatal("expected auth error, got nil")
	}
	if !strings.Contains(err.Error(), "AUTH_ERROR") {
		t.Errorf("expected AUTH_ERROR, got: %v", err)
	}
}

func TestCheckEmptyResultsAuthTransientFailure(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	var stderr bytes.Buffer
	err := CheckEmptyResultsAuth(context.Background(), client, &stderr)
	if err != nil {
		t.Fatalf("expected nil for transient failure, got: %v", err)
	}
	if !strings.Contains(stderr.String(), "Warning: credential check failed") {
		t.Errorf("expected stderr warning, got: %s", stderr.String())
	}
}

func TestCheckEmptyResultsAuthRateLimited(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))

	var stderr bytes.Buffer
	err := CheckEmptyResultsAuth(context.Background(), client, &stderr)
	if err != nil {
		t.Fatalf("expected nil for rate-limited probe, got: %v", err)
	}
	if !strings.Contains(stderr.String(), "Warning: credential check failed") {
		t.Errorf("expected stderr warning for 429, got: %s", stderr.String())
	}
}
