package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/endgame-build/jira-cli/internal/auth"
	cliErrors "github.com/endgame-build/jira-cli/internal/errors"
)

// newTestClient creates a Client whose base URL points at the httptest server.
// The server URL (http://127.0.0.1:PORT) is injected via withBaseURL so the
// HTTPS base URL doesn't conflict with httptest's plain HTTP server.
func newTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	creds := &auth.Credentials{
		Instance: "test.atlassian.net",
		User:     "user@example.com",
		Token:    "test-api-token",
	}
	return NewClient(creds, withBaseURL(serverURL+"/rest/api/3"))
}

func TestAuthTransport_InjectsHeaders(t *testing.T) {
	var gotAuth, gotContentType, gotAccept string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.Do(context.Background(), http.MethodGet, "myself", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify Basic auth header.
	wantCred := base64.StdEncoding.EncodeToString([]byte("user@example.com:test-api-token"))
	wantAuth := "Basic " + wantCred
	if gotAuth != wantAuth {
		t.Errorf("Authorization header = %q, want %q", gotAuth, wantAuth)
	}

	// Content-Type should not be set on GET requests (no body).
	if gotContentType != "" {
		t.Errorf("Content-Type on GET = %q, want empty (no body)", gotContentType)
	}

	if gotAccept != "application/json" {
		t.Errorf("Accept = %q, want %q", gotAccept, "application/json")
	}
}

func TestBaseURL_Construction(t *testing.T) {
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.Do(context.Background(), http.MethodGet, "issue/PROJ-123", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "/rest/api/3/issue/PROJ-123"
	if gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
}

func TestDo_Status200_DecodesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"displayName": "Alice"})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	var out struct {
		DisplayName string `json:"displayName"`
	}
	err := client.Do(context.Background(), http.MethodGet, "myself", nil, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.DisplayName != "Alice" {
		t.Errorf("displayName = %q, want %q", out.DisplayName, "Alice")
	}
}

func TestDo_Status201_DecodesBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"id": "10001", "key": "PROJ-1", "self": "https://x.atlassian.net/rest/api/3/issue/10001"})
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	var out struct {
		ID   string `json:"id"`
		Key  string `json:"key"`
		Self string `json:"self"`
	}
	err := client.Do(context.Background(), http.MethodPost, "issue", map[string]interface{}{"fields": map[string]string{"summary": "Test"}}, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Key != "PROJ-1" {
		t.Errorf("key = %q, want %q", out.Key, "PROJ-1")
	}
}

func TestDo_Status204_SkipsDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	// Pass a non-nil out pointer — must not error even though there's no body.
	var out struct{ Name string }
	err := client.Do(context.Background(), http.MethodPut, "issue/PROJ-1", map[string]string{"summary": "Updated"}, &out)
	if err != nil {
		t.Fatalf("unexpected error on 204: %v", err)
	}
}

func TestDo_Status204_NilOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	err := client.Do(context.Background(), http.MethodDelete, "issue/PROJ-1", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error on 204 with nil out: %v", err)
	}
}

func TestDo_ErrorStatus_ReturnsCLIError(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		wantCode cliErrors.ErrorCode
	}{
		{"400", http.StatusBadRequest, cliErrors.VALIDATION_ERROR},
		{"401", http.StatusUnauthorized, cliErrors.AUTH_ERROR},
		{"403", http.StatusForbidden, cliErrors.PERMISSION_DENIED},
		{"404", http.StatusNotFound, cliErrors.NOT_FOUND},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(`{"errorMessages":["something went wrong"]}`))
			}))
			defer srv.Close()

			client := newTestClient(t, srv.URL)
			err := client.Do(context.Background(), http.MethodGet, "issue/BAD", nil, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var cliErr *cliErrors.CLIError
			if !errors.As(err, &cliErr) {
				t.Fatalf("expected CLIError, got %T: %v", err, err)
			}
			if cliErr.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", cliErr.Code, tt.wantCode)
			}
			if !strings.Contains(cliErr.Message, "something went wrong") {
				t.Errorf("message = %q, want it to contain 'something went wrong'", cliErr.Message)
			}
		})
	}
}

func TestDo_SendsRequestBody(t *testing.T) {
	var gotBody map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"1","key":"PROJ-1","self":"x"}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	body := map[string]interface{}{
		"fields": map[string]string{
			"summary": "Test issue",
			"project": "PROJ",
		},
	}
	err := client.Do(context.Background(), http.MethodPost, "issue", body, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fields, ok := gotBody["fields"].(map[string]interface{})
	if !ok {
		t.Fatal("expected fields in request body")
	}
	if fields["summary"] != "Test issue" {
		t.Errorf("summary = %q, want %q", fields["summary"], "Test issue")
	}
}

func TestDo_NilBody_NoRequestBody(t *testing.T) {
	var gotContentLength int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentLength = r.ContentLength
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.Do(context.Background(), http.MethodGet, "myself", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// ContentLength is -1 or 0 when no body is sent.
	if gotContentLength > 0 {
		t.Errorf("expected no content body, got ContentLength=%d", gotContentLength)
	}
}

func TestDo_200_NilOut_NoDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"some":"data"}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	// nil out — should not attempt decode and not error.
	err := client.Do(context.Background(), http.MethodGet, "issue/PROJ-1", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error with nil out on 200: %v", err)
	}
}

func TestInstance_ReturnsHostname(t *testing.T) {
	creds := &auth.Credentials{
		Instance: "mysite.atlassian.net",
		User:     "user@example.com",
		Token:    "tok",
	}
	client := NewClient(creds)
	if got := client.Instance(); got != "mysite.atlassian.net" {
		t.Errorf("Instance() = %q, want %q", got, "mysite.atlassian.net")
	}
}

func TestBrowseURL(t *testing.T) {
	creds := &auth.Credentials{
		Instance: "mysite.atlassian.net",
		User:     "user@example.com",
		Token:    "tok",
	}
	client := NewClient(creds)
	got := client.BrowseURL("PROJ-123")
	want := "https://mysite.atlassian.net/browse/PROJ-123"
	if got != want {
		t.Errorf("BrowseURL() = %q, want %q", got, want)
	}
}

func TestWithTimeout(t *testing.T) {
	creds := &auth.Credentials{
		Instance: "mysite.atlassian.net",
		User:     "user@example.com",
		Token:    "tok",
	}
	client := NewClient(creds, WithTimeout(5*time.Second))
	if client.httpClient.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want %v", client.httpClient.Timeout, 5*time.Second)
	}
}

func TestDo_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	err := client.Do(ctx, http.MethodGet, "myself", nil, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

func TestDo_HTTPMethod_Preserved(t *testing.T) {
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var gotMethod string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				if method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
				} else {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{}`))
				}
			}))
			defer srv.Close()

			client := newTestClient(t, srv.URL)
			err := client.Do(context.Background(), method, "test", nil, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotMethod != method {
				t.Errorf("method = %q, want %q", gotMethod, method)
			}
		})
	}
}
