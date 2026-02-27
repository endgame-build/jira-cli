package user

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

func newTestUserSearchFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams) {
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

func userSearchHandler(t *testing.T, users []map[string]interface{}) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/user/search") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(users)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func sampleUsers() []map[string]interface{} {
	email := "jane@example.com"
	return []map[string]interface{}{
		{
			"accountId":    "5b10ac8d82e05b22cc7d4ef5",
			"displayName":  "Jane Doe",
			"emailAddress": email,
			"active":       true,
		},
		{
			"accountId":    "5b10a2844c20165700ede21g",
			"displayName":  "Jane Smith",
			"emailAddress": nil,
			"active":       true,
		},
	}
}

func TestUserSearch_Success(t *testing.T) {
	users := sampleUsers()
	f, tio := newTestUserSearchFactory(t, userSearchHandler(t, users))

	cmd := NewCmdSearch(f)
	cmd.SetArgs([]string{"jane"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Jane Doe") {
		t.Errorf("output should contain 'Jane Doe', got: %s", out)
	}
	if !strings.Contains(out, "Jane Smith") {
		t.Errorf("output should contain 'Jane Smith', got: %s", out)
	}
	if !strings.Contains(out, "jane@example.com") {
		t.Errorf("output should contain email, got: %s", out)
	}
	if !strings.Contains(out, "(hidden)") {
		t.Errorf("output should show '(hidden)' for null email, got: %s", out)
	}
}

func TestUserSearch_Empty(t *testing.T) {
	f, tio := newTestUserSearchFactory(t, userSearchHandler(t, []map[string]interface{}{}))

	cmd := NewCmdSearch(f)
	cmd.SetArgs([]string{"nonexistent"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "No users matching") {
		t.Errorf("expected empty message, got: %s", out)
	}
	if !strings.Contains(out, "nonexistent") {
		t.Errorf("expected query in message, got: %s", out)
	}
}

func TestUserSearch_NullEmail(t *testing.T) {
	users := []map[string]interface{}{
		{
			"accountId":    "5b10ac8d82e05b22cc7d4ef5",
			"displayName":  "No Email User",
			"emailAddress": nil,
			"active":       true,
		},
	}
	f, tio := newTestUserSearchFactory(t, userSearchHandler(t, users))

	cmd := NewCmdSearch(f)
	cmd.SetArgs([]string{"noemail"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "(hidden)") {
		t.Errorf("null email should show '(hidden)', got: %s", out)
	}
}

func TestUserSearch_JSON(t *testing.T) {
	users := sampleUsers()
	f, tio := newTestUserSearchFactory(t, userSearchHandler(t, users))
	f.OutputJSON = true

	cmd := NewCmdSearch(f)
	cmd.SetArgs([]string{"jane"})
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
	if envelope.Pagination.Total != nil {
		t.Errorf("pagination.total should be null for raw-array, got: %v", envelope.Pagination.Total)
	}
	if envelope.Pagination.HasNextPage {
		t.Errorf("has_next_page should be false when len(results) < limit")
	}

	// Verify camelCase field names from Jira API are preserved.
	var firstUser struct {
		AccountID   string  `json:"accountId"`
		DisplayName string  `json:"displayName"`
		Email       *string `json:"emailAddress"`
	}
	if err := json.Unmarshal(envelope.Data[0], &firstUser); err != nil {
		t.Fatalf("failed to parse first user: %v", err)
	}
	if firstUser.AccountID != "5b10ac8d82e05b22cc7d4ef5" {
		t.Errorf("accountId = %q, want 5b10ac8d82e05b22cc7d4ef5", firstUser.AccountID)
	}
	if firstUser.DisplayName != "Jane Doe" {
		t.Errorf("displayName = %q, want Jane Doe", firstUser.DisplayName)
	}
}

func TestUserSearch_JSON_HasNextPage(t *testing.T) {
	// Return exactly --limit results to trigger has_next_page=true.
	users := []map[string]interface{}{
		{
			"accountId":   "5b10ac8d82e05b22cc7d4ef5",
			"displayName": "User One",
			"active":      true,
		},
		{
			"accountId":   "5b10a2844c20165700ede21g",
			"displayName": "User Two",
			"active":      true,
		},
	}
	f, tio := newTestUserSearchFactory(t, userSearchHandler(t, users))
	f.OutputJSON = true

	cmd := NewCmdSearch(f)
	cmd.SetArgs([]string{"user", "--limit", "2"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var envelope struct {
		Pagination struct {
			HasNextPage bool `json:"has_next_page"`
			Limit       int  `json:"limit"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if !envelope.Pagination.HasNextPage {
		t.Error("has_next_page should be true when len(results) == limit")
	}
	if envelope.Pagination.Limit != 2 {
		t.Errorf("limit = %d, want 2", envelope.Pagination.Limit)
	}
}
