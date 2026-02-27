package user

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/auth"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/iostreams"
)

func newTestUserMeFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams) {
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

func myselfHandler(t *testing.T, user map[string]interface{}) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/myself") {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(user)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestUserMe_Success(t *testing.T) {
	email := "jane@example.com"
	user := map[string]interface{}{
		"accountId":    "5b10ac8d82e05b22cc7d4ef5",
		"displayName":  "Jane Doe",
		"emailAddress": email,
		"active":       true,
	}
	f, tio := newTestUserMeFactory(t, myselfHandler(t, user))

	cmd := NewCmdMe(f)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "5b10ac8d82e05b22cc7d4ef5") {
		t.Errorf("output should contain account ID, got: %s", out)
	}
	if !strings.Contains(out, "Jane Doe") {
		t.Errorf("output should contain display name, got: %s", out)
	}
	if !strings.Contains(out, "jane@example.com") {
		t.Errorf("output should contain email, got: %s", out)
	}
}

func TestUserMe_NullEmail(t *testing.T) {
	user := map[string]interface{}{
		"accountId":   "5b10ac8d82e05b22cc7d4ef5",
		"displayName": "Private User",
		"active":      true,
	}
	f, tio := newTestUserMeFactory(t, myselfHandler(t, user))

	cmd := NewCmdMe(f)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "(hidden)") {
		t.Errorf("null email should show '(hidden)', got: %s", out)
	}
	if !strings.Contains(out, "Private User") {
		t.Errorf("output should contain display name, got: %s", out)
	}
}

func TestUserMe_JSON(t *testing.T) {
	email := "jane@example.com"
	user := map[string]interface{}{
		"accountId":    "5b10ac8d82e05b22cc7d4ef5",
		"displayName":  "Jane Doe",
		"emailAddress": email,
		"active":       true,
	}
	f, tio := newTestUserMeFactory(t, myselfHandler(t, user))
	f.OutputJSON = true

	cmd := NewCmdMe(f)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result struct {
		AccountID    string  `json:"accountId"`
		DisplayName  string  `json:"displayName"`
		EmailAddress *string `json:"emailAddress"`
		Active       bool    `json:"active"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput: %s", err, out)
	}
	if result.AccountID != "5b10ac8d82e05b22cc7d4ef5" {
		t.Errorf("accountId = %q, want 5b10ac8d82e05b22cc7d4ef5", result.AccountID)
	}
	if result.DisplayName != "Jane Doe" {
		t.Errorf("displayName = %q, want Jane Doe", result.DisplayName)
	}
	if result.EmailAddress == nil || *result.EmailAddress != "jane@example.com" {
		t.Errorf("emailAddress = %v, want jane@example.com", result.EmailAddress)
	}
}
