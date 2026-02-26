package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	cliErrors "github.com/endgameio/jira-cli/internal/errors"
)

func TestRetry_429_RetriesAndSucceeds(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"errorMessages":["Rate limited"],"errors":{}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	var out struct {
		OK bool `json:"ok"`
	}
	err := client.Do(context.Background(), http.MethodGet, "myself", nil, &out)
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if !out.OK {
		t.Error("expected ok:true in response")
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestRetry_5xx_RetriesAndSucceeds(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"errorMessages":["Service Unavailable"],"errors":{}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	var out struct {
		OK bool `json:"ok"`
	}
	err := client.Do(context.Background(), http.MethodGet, "myself", nil, &out)
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}

func TestRetry_5xx_ExhaustsRetries(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"errorMessages":["Server error"],"errors":{}}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.Do(context.Background(), http.MethodGet, "myself", nil, nil)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	// maxRetries=3 + 1 initial = 4 total attempts.
	if got := attempts.Load(); got != 4 {
		t.Errorf("attempts = %d, want 4 (1 initial + 3 retries)", got)
	}
	// Should be a GENERAL_ERROR (5xx maps to that).
	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != cliErrors.GENERAL_ERROR {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.GENERAL_ERROR)
	}
}

func TestRetry_401_NoRetry(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errorMessages":["Unauthorized"],"errors":{}}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.Do(context.Background(), http.MethodGet, "myself", nil, nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (no retry on 401)", got)
	}
	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != cliErrors.AUTH_ERROR {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.AUTH_ERROR)
	}
}

func TestRetry_403_NoRetry(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errorMessages":["Forbidden"],"errors":{}}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.Do(context.Background(), http.MethodGet, "myself", nil, nil)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (no retry on 403)", got)
	}
}

func TestRetry_404_NoRetry(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["Not found"],"errors":{}}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.Do(context.Background(), http.MethodGet, "issue/BAD-1", nil, nil)
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1 (no retry on 404)", got)
	}
}

func TestDo_ErrorMapping_400(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errorMessages":[],"errors":{"summary":"Summary is required"}}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.Do(context.Background(), http.MethodPost, "issue", nil, nil)

	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != cliErrors.VALIDATION_ERROR {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.VALIDATION_ERROR)
	}
	if !strings.Contains(cliErr.Message, "Summary is required") {
		t.Errorf("message = %q, want it to contain 'Summary is required'", cliErr.Message)
	}
}

func TestDo_ErrorMapping_409(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"errorMessages":["Conflict"],"errors":{}}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.Do(context.Background(), http.MethodPut, "issue/PROJ-1", nil, nil)

	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != cliErrors.CONFLICT_ERROR {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.CONFLICT_ERROR)
	}
}

func TestDo_ErrorMapping_429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"errorMessages":["Rate limited"],"errors":{}}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv.URL)
	err := client.Do(context.Background(), http.MethodGet, "myself", nil, nil)

	// After exhausting retries on 429, we should get a RATE_LIMITED error.
	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != cliErrors.RATE_LIMITED {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.RATE_LIMITED)
	}
}

func TestDo_NetworkError_ConnectionRefused(t *testing.T) {
	// Create a server and immediately close it to get "connection refused".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srvURL := srv.URL
	srv.Close()

	client := newTestClient(t, srvURL)
	err := client.Do(context.Background(), http.MethodGet, "myself", nil, nil)
	if err == nil {
		t.Fatal("expected network error, got nil")
	}

	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != cliErrors.NETWORK_ERROR {
		t.Errorf("code = %q, want %q", cliErr.Code, cliErrors.NETWORK_ERROR)
	}
}

func TestDo_Existing_SuccessPathsStillWork(t *testing.T) {
	// Verify that the retry integration didn't break normal success paths.
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"200", http.StatusOK, `{"name":"test"}`},
		{"201", http.StatusCreated, `{"id":"1","key":"PROJ-1","self":"x"}`},
		{"204", http.StatusNoContent, ""},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				if tt.body != "" {
					w.Write([]byte(tt.body))
				}
			}))
			defer srv.Close()

			client := newTestClient(t, srv.URL)
			err := client.Do(context.Background(), http.MethodGet, "test", nil, nil)
			if err != nil {
				t.Fatalf("expected success for %d, got: %v", tt.status, err)
			}
		})
	}
}

func TestRetryPolicy_DirectCalls(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		status    int
		err       error
		wantRetry bool
	}{
		{"429 retries", 429, nil, true},
		{"500 retries", 500, nil, true},
		{"502 retries", 502, nil, true},
		{"503 retries", 503, nil, true},
		{"200 no retry", 200, nil, false},
		{"400 no retry", 400, nil, false},
		{"401 no retry", 401, nil, false},
		{"403 no retry", 403, nil, false},
		{"404 no retry", 404, nil, false},
		{"409 no retry", 409, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := newMockResponse(tt.status, "")
			retry, _ := retryPolicy(ctx, resp, tt.err)
			if retry != tt.wantRetry {
				t.Errorf("retryPolicy(status=%d) = %v, want %v", tt.status, retry, tt.wantRetry)
			}
		})
	}
}

func TestRetryPolicy_Timeout_NoRetry(t *testing.T) {
	ctx := context.Background()
	err := &mockTimeoutError{}
	retry, _ := retryPolicy(ctx, nil, err)
	if retry {
		t.Error("expected no retry on timeout error")
	}
}

// mockTimeoutError implements net.Error with Timeout() = true.
type mockTimeoutError struct{}

func (e *mockTimeoutError) Error() string   { return "mock timeout" }
func (e *mockTimeoutError) Timeout() bool   { return true }
func (e *mockTimeoutError) Temporary() bool { return false }
