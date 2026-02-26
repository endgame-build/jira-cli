package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListFields_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/field") {
			t.Errorf("path = %s, want suffix /field", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"id":     "summary",
				"key":    "summary",
				"name":   "Summary",
				"custom": false,
				"schema": map[string]interface{}{"type": "string", "system": "summary"},
			},
			{
				"id":     "customfield_10001",
				"key":    "customfield_10001",
				"name":   "Story Points",
				"custom": true,
				"schema": map[string]interface{}{"type": "number", "custom": "com.atlassian.jira.plugin.system.customfieldtypes:float"},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	fields, err := client.ListFields(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("len(fields) = %d, want 2", len(fields))
	}
	if fields[0].ID != "summary" {
		t.Errorf("fields[0].ID = %q, want %q", fields[0].ID, "summary")
	}
	if fields[0].Custom {
		t.Error("fields[0].Custom = true, want false")
	}
	if fields[0].Schema.System != "summary" {
		t.Errorf("fields[0].Schema.System = %q, want %q", fields[0].Schema.System, "summary")
	}
	if fields[1].ID != "customfield_10001" {
		t.Errorf("fields[1].ID = %q, want %q", fields[1].ID, "customfield_10001")
	}
	if !fields[1].Custom {
		t.Error("fields[1].Custom = false, want true")
	}
}

func TestListIssueTypes_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/issuetype") {
			t.Errorf("path = %s, want suffix /issuetype", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{"id": "1", "name": "Bug", "description": "A bug", "subtask": false, "iconUrl": "https://example.com/bug.png"},
			{"id": "2", "name": "Story", "description": "A story", "subtask": false, "iconUrl": "https://example.com/story.png"},
			{"id": "3", "name": "Sub-task", "description": "A subtask", "subtask": true, "iconUrl": "https://example.com/subtask.png"},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	types, err := client.ListIssueTypes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(types) != 3 {
		t.Fatalf("len(types) = %d, want 3", len(types))
	}
	if types[0].Name != "Bug" {
		t.Errorf("types[0].Name = %q, want %q", types[0].Name, "Bug")
	}
	if types[2].Subtask != true {
		t.Error("types[2].Subtask = false, want true")
	}
}

func TestListIssueTypesForProject_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/issuetype/project") {
			t.Errorf("path = %s, want /issuetype/project", r.URL.Path)
		}
		if r.URL.Query().Get("projectId") != "10001" {
			t.Errorf("projectId = %q, want %q", r.URL.Query().Get("projectId"), "10001")
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{"id": "1", "name": "Bug", "subtask": false},
			{"id": "2", "name": "Story", "subtask": false},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	types, err := client.ListIssueTypesForProject(context.Background(), "10001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(types) != 2 {
		t.Fatalf("len(types) = %d, want 2", len(types))
	}
	if types[0].Name != "Bug" {
		t.Errorf("types[0].Name = %q, want %q", types[0].Name, "Bug")
	}
}

func TestListStatuses_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/status") {
			t.Errorf("path = %s, want suffix /status", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"id":   "1",
				"name": "Open",
				"statusCategory": map[string]interface{}{
					"id":        2,
					"key":       "new",
					"colorName": "blue-gray",
					"name":      "To Do",
				},
			},
			{
				"id":   "3",
				"name": "In Progress",
				"statusCategory": map[string]interface{}{
					"id":        4,
					"key":       "indeterminate",
					"colorName": "yellow",
					"name":      "In Progress",
				},
			},
			{
				"id":          "6",
				"name":        "Closed",
				"description": "The issue is closed.",
				"statusCategory": map[string]interface{}{
					"id":        3,
					"key":       "done",
					"colorName": "green",
					"name":      "Done",
				},
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	statuses, err := client.ListStatuses(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 3 {
		t.Fatalf("len(statuses) = %d, want 3", len(statuses))
	}
	if statuses[0].Name != "Open" {
		t.Errorf("statuses[0].Name = %q, want %q", statuses[0].Name, "Open")
	}
	if statuses[0].StatusCategory == nil {
		t.Fatal("statuses[0].StatusCategory is nil")
	}
	if statuses[0].StatusCategory.Name != "To Do" {
		t.Errorf("statuses[0].StatusCategory.Name = %q, want %q", statuses[0].StatusCategory.Name, "To Do")
	}
	if statuses[2].Description != "The issue is closed." {
		t.Errorf("statuses[2].Description = %q, want %q", statuses[2].Description, "The issue is closed.")
	}
}

func TestListPriorities_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/priority") {
			t.Errorf("path = %s, want suffix /priority", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"id":          "1",
				"name":        "Highest",
				"iconUrl":     "https://example.com/highest.png",
				"description": "This problem will block progress.",
				"statusColor": "#d04437",
				"isDefault":   false,
			},
			{
				"id":          "3",
				"name":        "Medium",
				"iconUrl":     "https://example.com/medium.png",
				"description": "Has a moderate impact.",
				"statusColor": "#f6c342",
				"isDefault":   true,
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	priorities, err := client.ListPriorities(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(priorities) != 2 {
		t.Fatalf("len(priorities) = %d, want 2", len(priorities))
	}
	if priorities[0].Name != "Highest" {
		t.Errorf("priorities[0].Name = %q, want %q", priorities[0].Name, "Highest")
	}
	if priorities[0].StatusColor != "#d04437" {
		t.Errorf("priorities[0].StatusColor = %q, want %q", priorities[0].StatusColor, "#d04437")
	}
	if priorities[1].IsDefault != true {
		t.Error("priorities[1].IsDefault = false, want true")
	}
}

func TestListLabels_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/label") {
			t.Errorf("path = %s, want suffix /label", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"values":     []string{"backend", "frontend", "bug", "feature"},
			"startAt":    0,
			"maxResults": 50,
			"total":      4,
			"isLast":     true,
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	result, err := client.ListLabels(context.Background(), OffsetPaginationOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Values) != 4 {
		t.Fatalf("len(values) = %d, want 4", len(result.Values))
	}
	if result.Values[0] != "backend" {
		t.Errorf("values[0] = %q, want %q", result.Values[0], "backend")
	}
	if result.Total != 4 {
		t.Errorf("total = %d, want 4", result.Total)
	}
	if !result.IsLast {
		t.Error("isLast = false, want true")
	}
}

func TestListLabels_Pagination(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"values":     []string{"label-a", "label-b"},
			"startAt":    10,
			"maxResults": 2,
			"total":      20,
			"isLast":     false,
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	result, err := client.ListLabels(context.Background(), OffsetPaginationOptions{
		StartAt:    10,
		MaxResults: 2,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotQuery, "startAt=10") {
		t.Errorf("query = %q, want startAt=10", gotQuery)
	}
	if !strings.Contains(gotQuery, "maxResults=2") {
		t.Errorf("query = %q, want maxResults=2", gotQuery)
	}
	if result.IsLast {
		t.Error("isLast = true, want false")
	}
	if len(result.Values) != 2 {
		t.Fatalf("len(values) = %d, want 2", len(result.Values))
	}
}
