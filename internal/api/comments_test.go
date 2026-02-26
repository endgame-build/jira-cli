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

func TestListComments_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/issue/PROJ-123/comment") {
			t.Errorf("path = %s, want /issue/PROJ-123/comment", r.URL.Path)
		}
		if !strings.Contains(r.URL.RawQuery, "orderBy=-created") {
			t.Errorf("query = %s, want orderBy=-created", r.URL.RawQuery)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"comments": []map[string]interface{}{
				{
					"id":      "10042",
					"author":  map[string]interface{}{"accountId": "abc123", "displayName": "Alice"},
					"body":    map[string]interface{}{"type": "doc", "version": 1},
					"created": "2026-02-27T10:00:00.000+0000",
					"updated": "2026-02-27T10:00:00.000+0000",
				},
				{
					"id":      "10041",
					"author":  map[string]interface{}{"accountId": "def456", "displayName": "Bob"},
					"body":    map[string]interface{}{"type": "doc", "version": 1},
					"created": "2026-02-26T10:00:00.000+0000",
					"updated": "2026-02-26T10:00:00.000+0000",
				},
			},
			"maxResults": 25,
			"total":      2,
			"startAt":    0,
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	page, err := client.ListComments(context.Background(), "PROJ-123", OffsetPaginationOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Comments) != 2 {
		t.Fatalf("len(comments) = %d, want 2", len(page.Comments))
	}
	if page.Comments[0].ID != "10042" {
		t.Errorf("comments[0].ID = %q, want %q", page.Comments[0].ID, "10042")
	}
	if page.Total != 2 {
		t.Errorf("total = %d, want 2", page.Total)
	}
}

func TestListComments_Pagination(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"comments":   []interface{}{},
			"maxResults": 10,
			"total":      50,
			"startAt":    20,
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.ListComments(context.Background(), "PROJ-1", OffsetPaginationOptions{
		StartAt:    20,
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(gotQuery, "startAt=20") {
		t.Errorf("query = %q, want startAt=20", gotQuery)
	}
	if !strings.Contains(gotQuery, "maxResults=10") {
		t.Errorf("query = %q, want maxResults=10", gotQuery)
	}
}

func TestListComments_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["Issue does not exist or you do not have permission to see it."]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.ListComments(context.Background(), "NOPE-999", OffsetPaginationOptions{})
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

func TestGetComment_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/issue/PROJ-123/comment/10042") {
			t.Errorf("path = %s, want suffix /issue/PROJ-123/comment/10042", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "10042",
			"author":  map[string]interface{}{"accountId": "abc123", "displayName": "Alice"},
			"body":    map[string]interface{}{"type": "doc", "version": 1},
			"created": "2026-02-27T10:00:00.000+0000",
			"updated": "2026-02-27T10:00:00.000+0000",
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	comment, err := client.GetComment(context.Background(), "PROJ-123", "10042")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comment.ID != "10042" {
		t.Errorf("ID = %q, want %q", comment.ID, "10042")
	}
	if comment.Author.DisplayName != "Alice" {
		t.Errorf("Author.DisplayName = %q, want %q", comment.Author.DisplayName, "Alice")
	}
}

func TestGetComment_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["Comment does not exist"]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.GetComment(context.Background(), "PROJ-123", "99999")
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

func TestAddComment_Success(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/issue/PROJ-123/comment") {
			t.Errorf("path = %s, want suffix /issue/PROJ-123/comment", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "10043",
			"author":  map[string]interface{}{"accountId": "abc123", "displayName": "Alice"},
			"body":    gotBody["body"],
			"created": "2026-02-27T12:00:00.000+0000",
			"updated": "2026-02-27T12:00:00.000+0000",
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	adfBody := map[string]interface{}{
		"type":    "doc",
		"version": 1,
		"content": []map[string]interface{}{
			{"type": "paragraph", "content": []map[string]interface{}{
				{"type": "text", "text": "Hello world"},
			}},
		},
	}
	comment, err := client.AddComment(context.Background(), "PROJ-123", adfBody)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comment.ID != "10043" {
		t.Errorf("ID = %q, want %q", comment.ID, "10043")
	}

	// Verify request body shape.
	if _, ok := gotBody["body"]; !ok {
		t.Error("expected 'body' in request payload")
	}
}

func TestAddComment_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["Issue does not exist or you do not have permission to see it."]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.AddComment(context.Background(), "NOPE-999", map[string]interface{}{})
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

func TestAddComment_403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errorMessages":["You do not have permission to add comments"]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.AddComment(context.Background(), "PROJ-123", map[string]interface{}{})
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

func TestAddComment_413(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		w.Write([]byte(`{"errorMessages":["Comment body exceeds maximum allowed size"]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.AddComment(context.Background(), "PROJ-123", map[string]interface{}{})
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
	if cliErr.Suggestion == "" {
		t.Error("expected a suggestion for 413 error")
	}
}

func TestUpdateComment_Success(t *testing.T) {
	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/issue/PROJ-123/comment/10042") {
			t.Errorf("path = %s, want suffix /issue/PROJ-123/comment/10042", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "10042",
			"author":  map[string]interface{}{"accountId": "abc123", "displayName": "Alice"},
			"body":    gotBody["body"],
			"created": "2026-02-27T10:00:00.000+0000",
			"updated": "2026-02-27T12:00:00.000+0000",
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	adfBody := map[string]interface{}{
		"type":    "doc",
		"version": 1,
		"content": []map[string]interface{}{
			{"type": "paragraph", "content": []map[string]interface{}{
				{"type": "text", "text": "Updated comment"},
			}},
		},
	}
	comment, err := client.UpdateComment(context.Background(), "PROJ-123", "10042", adfBody)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comment.ID != "10042" {
		t.Errorf("ID = %q, want %q", comment.ID, "10042")
	}

	// Verify request body.
	if _, ok := gotBody["body"]; !ok {
		t.Error("expected 'body' in request payload")
	}
}

func TestUpdateComment_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["Comment does not exist"]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.UpdateComment(context.Background(), "PROJ-123", "99999", map[string]interface{}{})
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

func TestUpdateComment_403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errorMessages":["You do not have permission to edit this comment"]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.UpdateComment(context.Background(), "PROJ-123", "10042", map[string]interface{}{})
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

func TestDeleteComment_Success(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		if !strings.HasSuffix(r.URL.Path, "/issue/PROJ-123/comment/10042") {
			t.Errorf("path = %s, want suffix /issue/PROJ-123/comment/10042", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.DeleteComment(context.Background(), "PROJ-123", "10042")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
}

func TestDeleteComment_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["Comment does not exist"]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.DeleteComment(context.Background(), "PROJ-123", "99999")
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

func TestDeleteComment_403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errorMessages":["You do not have permission to delete this comment"]}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.DeleteComment(context.Background(), "PROJ-123", "10042")
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
