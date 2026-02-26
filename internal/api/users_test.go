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

func TestGetMyself_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/myself") {
			t.Errorf("path = %s, want suffix /myself", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accountId":    "abc123",
			"displayName":  "Test User",
			"emailAddress": "test@example.com",
			"active":       true,
			"timeZone":     "UTC",
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	user, err := client.GetMyself(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.AccountID != "abc123" {
		t.Errorf("accountId = %q, want %q", user.AccountID, "abc123")
	}
	if user.DisplayName != "Test User" {
		t.Errorf("displayName = %q, want %q", user.DisplayName, "Test User")
	}
	if user.EmailAddress == nil || *user.EmailAddress != "test@example.com" {
		t.Errorf("emailAddress = %v, want %q", user.EmailAddress, "test@example.com")
	}
	if !user.Active {
		t.Errorf("active = false, want true")
	}
}

func TestGetMyself_NullEmail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accountId":   "abc123",
			"displayName": "Private User",
			"active":      true,
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	user, err := client.GetMyself(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.EmailAddress != nil {
		t.Errorf("emailAddress = %v, want nil", user.EmailAddress)
	}
}

func TestGetMyself_401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Client must be authenticated to access this resource."},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.GetMyself(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != cliErrors.AUTH_ERROR {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.AUTH_ERROR)
	}
}

func TestSearchUsers_Success(t *testing.T) {
	var gotQuery, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotQuery = r.URL.Query().Get("query")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"accountId":    "user1",
				"displayName":  "Alice Smith",
				"emailAddress": "alice@example.com",
				"active":       true,
			},
			{
				"accountId":   "user2",
				"displayName": "Bob Smith",
				"active":      true,
			},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	users, err := client.SearchUsers(context.Background(), "Smith", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if gotQuery != "Smith" {
		t.Errorf("query param = %q, want %q", gotQuery, "Smith")
	}
	if len(users) != 2 {
		t.Fatalf("len(users) = %d, want 2", len(users))
	}
	if users[0].AccountID != "user1" {
		t.Errorf("users[0].accountId = %q, want %q", users[0].AccountID, "user1")
	}
	if users[1].DisplayName != "Bob Smith" {
		t.Errorf("users[1].displayName = %q, want %q", users[1].DisplayName, "Bob Smith")
	}
}

func TestSearchUsers_WithPagination(t *testing.T) {
	var gotStartAt, gotMaxResults string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotStartAt = r.URL.Query().Get("startAt")
		gotMaxResults = r.URL.Query().Get("maxResults")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{"accountId": "user3", "displayName": "Charlie", "active": true},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	users, err := client.SearchUsers(context.Background(), "test", 10, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotStartAt != "10" {
		t.Errorf("startAt = %q, want %q", gotStartAt, "10")
	}
	if gotMaxResults != "5" {
		t.Errorf("maxResults = %q, want %q", gotMaxResults, "5")
	}
	if len(users) != 1 {
		t.Fatalf("len(users) = %d, want 1", len(users))
	}
}

func TestSearchUsers_NoPaginationParams(t *testing.T) {
	var gotStartAt, gotMaxResults string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotStartAt = r.URL.Query().Get("startAt")
		gotMaxResults = r.URL.Query().Get("maxResults")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]interface{}{})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.SearchUsers(context.Background(), "nobody", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotStartAt != "" {
		t.Errorf("startAt should be absent, got %q", gotStartAt)
	}
	if gotMaxResults != "" {
		t.Errorf("maxResults should be absent, got %q", gotMaxResults)
	}
}

func TestSearchUsers_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	users, err := client.SearchUsers(context.Background(), "nonexistent", 0, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("len(users) = %d, want 0", len(users))
	}
}

func TestSearchUsers_400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"query parameter is required"},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.SearchUsers(context.Background(), "", 0, 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != cliErrors.VALIDATION_ERROR {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.VALIDATION_ERROR)
	}
}

func TestSearchUsers_403(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"You don't have permission to browse users"},
		})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.SearchUsers(context.Background(), "test", 0, 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != cliErrors.PERMISSION_DENIED {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.PERMISSION_DENIED)
	}
}

func TestSearchUsers_PathAndQuery(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	_, err := client.SearchUsers(context.Background(), "test query", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/user/search") {
		t.Errorf("path = %q, want suffix /user/search", gotPath)
	}
}
