package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/config"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/iostreams"
)

// newTestStatusFactory creates a Factory pre-loaded with a stored profile and token.
func newTestStatusFactory(t *testing.T) (*factory.Factory, *iostreams.TestIOStreams) {
	t.Helper()
	keyring.MockInit()

	tio := iostreams.Test()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	// Store a profile so the status command has credentials to resolve.
	type profileSetter interface {
		SetProfile(name, instance, user string)
		SetActiveProfile(name string) error
		config.Config
	}
	pc := cfg.(profileSetter)
	pc.SetProfile("default", "mysite.atlassian.net", "user@example.com")
	if err := pc.SetActiveProfile("default"); err != nil {
		t.Fatalf("set active: %v", err)
	}
	if err := pc.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}

	// Store token in mock keyring.
	if err := keyring.Set("jira-cli", "default-token", "tok123"); err != nil {
		t.Fatalf("set keyring: %v", err)
	}

	f := factory.NewTestFactory(tio.IOStreams, cfg, nil)
	return f, tio
}

func TestStatusValidToken(t *testing.T) {
	f, tio := newTestStatusFactory(t)
	email := "user@example.com"

	srv := httptest.NewServer(myselfHandler(api.User{
		AccountID:    "abc123",
		DisplayName:  "Jane Doe",
		EmailAddress: &email,
		Active:       true,
	}))
	defer srv.Close()

	opts := &StatusOptions{
		Factory:    f,
		clientOpts: []api.ClientOption{api.WithBaseURL(srv.URL)},
	}

	if err := runStatus(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Profile:") {
		t.Errorf("output missing Profile line: %q", out)
	}
	if !strings.Contains(out, "mysite.atlassian.net") {
		t.Errorf("output missing instance: %q", out)
	}
	if !strings.Contains(out, "user@example.com") {
		t.Errorf("output missing user: %q", out)
	}
	if !strings.Contains(out, "valid") {
		t.Errorf("output missing 'valid' indicator: %q", out)
	}
}

func TestStatusInvalidToken(t *testing.T) {
	f, tio := newTestStatusFactory(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"errorMessages":["Client must be authenticated"]}`))
	}))
	defer srv.Close()

	opts := &StatusOptions{
		Factory:    f,
		clientOpts: []api.ClientOption{api.WithBaseURL(srv.URL)},
	}

	// Should NOT return an error — invalid token is reported, not an error.
	if err := runStatus(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "invalid") {
		t.Errorf("output missing 'invalid' indicator: %q", out)
	}
	if !strings.Contains(out, "mysite.atlassian.net") {
		t.Errorf("output missing instance: %q", out)
	}
}

func TestStatusNoCredentials(t *testing.T) {
	keyring.MockInit()

	tio := iostreams.Test()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	// No profile stored — auth resolution should fail.
	f := factory.NewTestFactory(tio.IOStreams, cfg, nil)

	opts := &StatusOptions{
		Factory: f,
	}

	err = runStatus(opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.AUTH_ERROR {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.AUTH_ERROR)
	}
}

func TestStatusJSONShape(t *testing.T) {
	f, tio := newTestStatusFactory(t)
	f.OutputJSON = true
	email := "user@example.com"

	srv := httptest.NewServer(myselfHandler(api.User{
		AccountID:    "abc123",
		DisplayName:  "Jane Doe",
		EmailAddress: &email,
		Active:       true,
	}))
	defer srv.Close()

	opts := &StatusOptions{
		Factory:    f,
		clientOpts: []api.ClientOption{api.WithBaseURL(srv.URL)},
	}

	if err := runStatus(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	for _, field := range []string{"profile", "instance", "user", "token_valid"} {
		if _, ok := result[field]; !ok {
			t.Errorf("missing JSON field %q", field)
		}
	}

	if result["token_valid"] != true {
		t.Errorf("token_valid = %v, want true", result["token_valid"])
	}
	if result["instance"] != "mysite.atlassian.net" {
		t.Errorf("instance = %v, want %q", result["instance"], "mysite.atlassian.net")
	}
	if result["user"] != "user@example.com" {
		t.Errorf("user = %v, want %q", result["user"], "user@example.com")
	}
	if result["profile"] != "default" {
		t.Errorf("profile = %v, want %q", result["profile"], "default")
	}
}

func TestStatusJSONInvalidToken(t *testing.T) {
	f, tio := newTestStatusFactory(t)
	f.OutputJSON = true

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"errorMessages":["Client must be authenticated"]}`))
	}))
	defer srv.Close()

	opts := &StatusOptions{
		Factory:    f,
		clientOpts: []api.ClientOption{api.WithBaseURL(srv.URL)},
	}

	if err := runStatus(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if result["token_valid"] != false {
		t.Errorf("token_valid = %v, want false", result["token_valid"])
	}
}

func TestStatusNullEmail(t *testing.T) {
	f, tio := newTestStatusFactory(t)

	srv := httptest.NewServer(myselfHandler(api.User{
		AccountID:   "abc123",
		DisplayName: "Jane Doe",
		Active:      true,
	}))
	defer srv.Close()

	opts := &StatusOptions{
		Factory:    f,
		clientOpts: []api.ClientOption{api.WithBaseURL(srv.URL)},
	}

	if err := runStatus(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "(email hidden)") {
		t.Errorf("output should show '(email hidden)' for null email: %q", out)
	}
}
