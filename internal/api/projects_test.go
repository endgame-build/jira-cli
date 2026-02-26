package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cliErrors "github.com/endgameio/jira-cli/internal/errors"
)

func TestListProjects_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/project/search") {
			t.Errorf("path = %s, want suffix /project/search", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"values": []map[string]interface{}{
				{
					"id":   "10001",
					"key":  "PROJ",
					"name": "Project Alpha",
					"lead": map[string]interface{}{"accountId": "abc123", "displayName": "Alice"},
				},
				{
					"id":   "10002",
					"key":  "BETA",
					"name": "Project Beta",
					"lead": map[string]interface{}{"accountId": "def456", "displayName": "Bob"},
				},
			},
			"startAt":    0,
			"maxResults": 50,
			"total":      2,
			"isLast":     true,
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	result, err := client.ListProjects(context.Background(), OffsetPaginationOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Values) != 2 {
		t.Fatalf("len(values) = %d, want 2", len(result.Values))
	}
	if result.Values[0].Key != "PROJ" {
		t.Errorf("values[0].Key = %q, want %q", result.Values[0].Key, "PROJ")
	}
	if result.Values[1].Name != "Project Beta" {
		t.Errorf("values[1].Name = %q, want %q", result.Values[1].Name, "Project Beta")
	}
	if result.Total != 2 {
		t.Errorf("total = %d, want 2", result.Total)
	}
	if !result.IsLast {
		t.Error("isLast = false, want true")
	}
}

func TestListProjects_Pagination(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"values":     []interface{}{},
			"startAt":    10,
			"maxResults": 5,
			"total":      20,
			"isLast":     false,
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	result, err := client.ListProjects(context.Background(), OffsetPaginationOptions{
		StartAt:    10,
		MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotQuery, "startAt=10") {
		t.Errorf("query = %q, want startAt=10", gotQuery)
	}
	if !strings.Contains(gotQuery, "maxResults=5") {
		t.Errorf("query = %q, want maxResults=5", gotQuery)
	}
	if result.IsLast {
		t.Error("isLast = true, want false")
	}
}

func TestGetProject_ByKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/project/PROJ") {
			t.Errorf("path = %s, want suffix /project/PROJ", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":             "10001",
			"key":            "PROJ",
			"name":           "Project Alpha",
			"description":    "A test project",
			"projectTypeKey": "software",
			"simplified":     false,
			"style":          "classic",
			"lead":           map[string]interface{}{"accountId": "abc123", "displayName": "Alice"},
			"issueTypes": []map[string]interface{}{
				{"id": "1", "name": "Bug"},
				{"id": "2", "name": "Story"},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	project, err := client.GetProject(context.Background(), "PROJ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if project.Key != "PROJ" {
		t.Errorf("Key = %q, want %q", project.Key, "PROJ")
	}
	if project.Name != "Project Alpha" {
		t.Errorf("Name = %q, want %q", project.Name, "Project Alpha")
	}
	if project.Description != "A test project" {
		t.Errorf("Description = %q, want %q", project.Description, "A test project")
	}
	if project.Lead == nil {
		t.Fatal("Lead is nil")
	}
	if project.Lead.DisplayName != "Alice" {
		t.Errorf("Lead.DisplayName = %q, want %q", project.Lead.DisplayName, "Alice")
	}
	if len(project.IssueTypes) != 2 {
		t.Fatalf("len(IssueTypes) = %d, want 2", len(project.IssueTypes))
	}
}

func TestGetProject_ByNumericID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/project/10001") {
			t.Errorf("path = %s, want suffix /project/10001", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":   "10001",
			"key":  "PROJ",
			"name": "Project Alpha",
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	project, err := client.GetProject(context.Background(), "10001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if project.ID != "10001" {
		t.Errorf("ID = %q, want %q", project.ID, "10001")
	}
}

func TestGetProject_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["No project could be found with key 'NOPE'."]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.GetProject(context.Background(), "NOPE")
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
	// Verify project key is in context.
	if cliErr.Context == nil {
		t.Fatal("expected context with project key")
	}
	if cliErr.Context["key"] != "NOPE" {
		t.Errorf("context[key] = %v, want %q", cliErr.Context["key"], "NOPE")
	}
}
