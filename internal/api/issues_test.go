package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cliErrors "github.com/endgame-build/jira-cli/internal/errors"
)

func TestGetIssue_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/issue/PROJ-123") {
			t.Errorf("path = %s, want suffix /issue/PROJ-123", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":   "10001",
			"key":  "PROJ-123",
			"self": "https://test.atlassian.net/rest/api/3/issue/10001",
			"fields": map[string]interface{}{
				"summary": "Test issue",
				"status":  map[string]interface{}{"id": "1", "name": "Open"},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	issue, err := client.GetIssue(context.Background(), "PROJ-123", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.Key != "PROJ-123" {
		t.Errorf("key = %q, want %q", issue.Key, "PROJ-123")
	}
	if issue.Fields.Summary != "Test issue" {
		t.Errorf("summary = %q, want %q", issue.Fields.Summary, "Test issue")
	}
}

func TestGetIssue_WithOptions(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "10001", "key": "PROJ-1", "self": "x",
			"fields": map[string]interface{}{"summary": "s"},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.GetIssue(context.Background(), "PROJ-1", &GetIssueOptions{
		Fields: []string{"summary", "status"},
		Expand: []string{"renderedFields"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotQuery, "fields=summary,status") {
		t.Errorf("query = %q, want fields=summary,status", gotQuery)
	}
	if !strings.Contains(gotQuery, "expand=renderedFields") {
		t.Errorf("query = %q, want expand=renderedFields", gotQuery)
	}
}

func TestGetIssue_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["Issue does not exist or you do not have permission to see it."]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.GetIssue(context.Background(), "NOPE-999", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Code != cliErrors.NOT_FOUND {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.NOT_FOUND)
	}
}

func TestCreateIssue_Success(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{
			"id":   "10042",
			"key":  "PROJ-42",
			"self": "https://test.atlassian.net/rest/api/3/issue/10042",
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	input := &CreateIssueInput{
		Fields: map[string]interface{}{
			"summary":   "New issue",
			"project":   map[string]string{"key": "PROJ"},
			"issuetype": map[string]string{"name": "Task"},
		},
	}
	created, err := client.CreateIssue(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created.Key != "PROJ-42" {
		t.Errorf("key = %q, want %q", created.Key, "PROJ-42")
	}
	if created.ID != "10042" {
		t.Errorf("id = %q, want %q", created.ID, "10042")
	}

	// Verify request body was sent correctly.
	fields, ok := gotBody["fields"].(map[string]interface{})
	if !ok {
		t.Fatal("expected fields in request body")
	}
	if fields["summary"] != "New issue" {
		t.Errorf("summary = %v, want %q", fields["summary"], "New issue")
	}
}

func TestCreateIssue_400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errorMessages":[],"errors":{"summary":"Field 'summary' is required"}}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.CreateIssue(context.Background(), &CreateIssueInput{
		Fields: map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Code != cliErrors.VALIDATION_ERROR {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.VALIDATION_ERROR)
	}
	if !strings.Contains(cliErr.Message, "summary") {
		t.Errorf("message = %q, want it to contain 'summary'", cliErr.Message)
	}
}

func TestEditIssue_Success(t *testing.T) {
	var gotMethod string
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.EditIssue(context.Background(), "PROJ-1", &EditIssueInput{
		Fields: map[string]interface{}{
			"summary": "Updated summary",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	fields, ok := gotBody["fields"].(map[string]interface{})
	if !ok {
		t.Fatal("expected fields in request body")
	}
	if fields["summary"] != "Updated summary" {
		t.Errorf("summary = %v, want %q", fields["summary"], "Updated summary")
	}
}

func TestEditIssue_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["Issue does not exist"]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.EditIssue(context.Background(), "NOPE-1", &EditIssueInput{
		Fields: map[string]interface{}{"summary": "x"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Code != cliErrors.NOT_FOUND {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.NOT_FOUND)
	}
}

func TestDeleteIssue_Success(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.DeleteIssue(context.Background(), "PROJ-1", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	if !strings.Contains(gotPath, "deleteSubtasks=true") {
		t.Errorf("path = %q, want deleteSubtasks=true", gotPath)
	}
}

func TestDeleteIssue_WithoutSubtasks(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.DeleteIssue(context.Background(), "PROJ-1", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotPath, "deleteSubtasks=false") {
		t.Errorf("path = %q, want deleteSubtasks=false", gotPath)
	}
}

func TestDeleteIssue_403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errorMessages":["You do not have permission to delete issues"]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.DeleteIssue(context.Background(), "PROJ-1", true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Code != cliErrors.PERMISSION_DENIED {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.PERMISSION_DENIED)
	}
}

func TestAssignIssue_Assign(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/assignee") {
			t.Errorf("path = %s, want suffix /assignee", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	accountID := "5b10ac8d82e05b22cc7d4ef5"
	err := client.AssignIssue(context.Background(), "PROJ-1", &accountID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody["accountId"] != accountID {
		t.Errorf("accountId = %v, want %q", gotBody["accountId"], accountID)
	}
}

func TestAssignIssue_Unassign(t *testing.T) {
	var gotRawBody json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotRawBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.AssignIssue(context.Background(), "PROJ-1", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify the body contains {"accountId": null}.
	if !strings.Contains(string(gotRawBody), `"accountId":null`) {
		t.Errorf("body = %s, want accountId:null", string(gotRawBody))
	}
}

func TestGetTransitions_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/transitions") {
			t.Errorf("path = %s, want suffix /transitions", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"transitions": []map[string]interface{}{
				{"id": "11", "name": "To Do", "to": map[string]interface{}{"id": "1", "name": "To Do"}},
				{"id": "21", "name": "In Progress", "to": map[string]interface{}{"id": "2", "name": "In Progress"}},
				{"id": "31", "name": "Done", "to": map[string]interface{}{"id": "3", "name": "Done"}},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	transitions, err := client.GetTransitions(context.Background(), "PROJ-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(transitions) != 3 {
		t.Fatalf("len(transitions) = %d, want 3", len(transitions))
	}
	if transitions[1].Name != "In Progress" {
		t.Errorf("transitions[1].Name = %q, want %q", transitions[1].Name, "In Progress")
	}
	if transitions[1].To.Name != "In Progress" {
		t.Errorf("transitions[1].To.Name = %q, want %q", transitions[1].To.Name, "In Progress")
	}
}

func TestGetTransitions_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["Issue does not exist"]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.GetTransitions(context.Background(), "NOPE-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Code != cliErrors.NOT_FOUND {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.NOT_FOUND)
	}
}

func TestDoTransition_Success(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.DoTransition(context.Background(), "PROJ-1", &DoTransitionInput{
		Transition: TransitionRef{ID: "21"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	transition, ok := gotBody["transition"].(map[string]interface{})
	if !ok {
		t.Fatal("expected transition in request body")
	}
	if transition["id"] != "21" {
		t.Errorf("transition.id = %v, want %q", transition["id"], "21")
	}
}

func TestGetCreateMeta_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "createmeta/PROJ/issuetypes") {
			t.Errorf("path = %s, want createmeta/PROJ/issuetypes", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"issueTypes": []map[string]interface{}{
				{"id": "10001", "name": "Bug", "subtask": false},
				{"id": "10002", "name": "Task", "subtask": false},
				{"id": "10003", "name": "Sub-task", "subtask": true},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	meta, err := client.GetCreateMeta(context.Background(), "PROJ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(meta.IssueTypes) != 3 {
		t.Fatalf("len(issueTypes) = %d, want 3", len(meta.IssueTypes))
	}
	if meta.IssueTypes[0].Name != "Bug" {
		t.Errorf("issueTypes[0].Name = %q, want %q", meta.IssueTypes[0].Name, "Bug")
	}
	if meta.IssueTypes[2].Subtask != true {
		t.Error("issueTypes[2].Subtask = false, want true")
	}
}

func TestGetCreateMeta_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["Project does not exist"]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.GetCreateMeta(context.Background(), "NOPE")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Code != cliErrors.NOT_FOUND {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.NOT_FOUND)
	}
}

func TestSearchIssues_Success(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"issues": []map[string]interface{}{
				{
					"id": "10001", "key": "PROJ-1", "self": "x",
					"fields": map[string]interface{}{"summary": "First issue"},
				},
				{
					"id": "10002", "key": "PROJ-2", "self": "x",
					"fields": map[string]interface{}{"summary": "Second issue"},
				},
			},
			"nextPageToken": "abc123",
			"isLast":        false,
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	results, err := client.SearchIssues(context.Background(), &SearchOptions{
		JQL:        "project = PROJ",
		MaxResults: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results.Issues) != 2 {
		t.Fatalf("len(issues) = %d, want 2", len(results.Issues))
	}
	if results.Issues[0].Key != "PROJ-1" {
		t.Errorf("issues[0].key = %q, want %q", results.Issues[0].Key, "PROJ-1")
	}
	if results.NextPageToken != "abc123" {
		t.Errorf("nextPageToken = %q, want %q", results.NextPageToken, "abc123")
	}
	if results.IsLast {
		t.Error("isLast = true, want false")
	}
}

func TestSearchIssues_DefaultsFieldsToAll(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"issues": []interface{}{},
			"isLast": true,
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.SearchIssues(context.Background(), &SearchOptions{
		JQL: "project = PROJ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify fields defaults to ["*all"].
	fields, ok := gotBody["fields"].([]interface{})
	if !ok {
		t.Fatalf("expected fields in body, got %T", gotBody["fields"])
	}
	if len(fields) != 1 || fields[0] != "*all" {
		t.Errorf("fields = %v, want [*all]", fields)
	}
}

func TestSearchIssues_ExplicitFields(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"issues": []interface{}{},
			"isLast": true,
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.SearchIssues(context.Background(), &SearchOptions{
		JQL:    "project = PROJ",
		Fields: []string{"summary", "status"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fields, ok := gotBody["fields"].([]interface{})
	if !ok {
		t.Fatalf("expected fields in body, got %T", gotBody["fields"])
	}
	if len(fields) != 2 {
		t.Fatalf("len(fields) = %d, want 2", len(fields))
	}
	if fields[0] != "summary" || fields[1] != "status" {
		t.Errorf("fields = %v, want [summary, status]", fields)
	}
}

func TestSearchIssues_WithPagination(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"issues": []interface{}{},
			"isLast": true,
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.SearchIssues(context.Background(), &SearchOptions{
		JQL:           "project = PROJ",
		MaxResults:    25,
		NextPageToken: "tok123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotBody["maxResults"].(float64) != 25 {
		t.Errorf("maxResults = %v, want 25", gotBody["maxResults"])
	}
	if gotBody["nextPageToken"] != "tok123" {
		t.Errorf("nextPageToken = %v, want %q", gotBody["nextPageToken"], "tok123")
	}
}

func TestSearchIssues_400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errorMessages":["Error in the JQL Query"]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.SearchIssues(context.Background(), &SearchOptions{
		JQL: "invalid jql !!!",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Code != cliErrors.VALIDATION_ERROR {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.VALIDATION_ERROR)
	}
}

func TestEditIssue_409_Conflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"errorMessages":["Issue has been modified"]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.EditIssue(context.Background(), "PROJ-1", &EditIssueInput{
		Fields: map[string]interface{}{"summary": "x"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Code != cliErrors.CONFLICT_ERROR {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.CONFLICT_ERROR)
	}
}

func TestAssignIssue_403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errorMessages":["You do not have permission to assign issues"]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	accountID := "5b10ac8d82e05b22cc7d4ef5"
	err := client.AssignIssue(context.Background(), "PROJ-1", &accountID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Code != cliErrors.PERMISSION_DENIED {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.PERMISSION_DENIED)
	}
}

func TestDoTransition_WithUpdate(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	// Build update with comment via json.RawMessage.
	commentOps, _ := json.Marshal([]map[string]interface{}{
		{"add": map[string]interface{}{
			"body": map[string]interface{}{
				"type":    "doc",
				"version": 1,
				"content": []map[string]interface{}{
					{"type": "paragraph", "content": []map[string]interface{}{
						{"type": "text", "text": "Resolved"},
					}},
				},
			},
		}},
	})

	err := client.DoTransition(context.Background(), "PROJ-1", &DoTransitionInput{
		Transition: TransitionRef{ID: "31"},
		Update: map[string]json.RawMessage{
			"comment": commentOps,
		},
		Fields: map[string]interface{}{
			"resolution": map[string]string{"name": "Fixed"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify transition ID.
	tr, ok := gotBody["transition"].(map[string]interface{})
	if !ok {
		t.Fatal("expected transition in body")
	}
	if tr["id"] != "31" {
		t.Errorf("transition.id = %v, want %q", tr["id"], "31")
	}

	// Verify update was included.
	if _, ok := gotBody["update"]; !ok {
		t.Error("expected update in request body")
	}

	// Verify fields was included.
	if _, ok := gotBody["fields"]; !ok {
		t.Error("expected fields in request body")
	}
}
