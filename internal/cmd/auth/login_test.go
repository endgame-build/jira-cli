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

	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/config"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/iostreams"
)

func newTestLoginFactory(t *testing.T) (*factory.Factory, *iostreams.TestIOStreams, string) {
	t.Helper()
	keyring.MockInit()

	tio := iostreams.Test()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	f := factory.NewTestFactory(tio.IOStreams, cfg, nil)
	return f, tio, cfgPath
}

func myselfHandler(user api.User) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// WithBaseURL replaces the full base URL, so path arrives as /myself (not /rest/api/3/myself).
		if r.URL.Path != "/myself" {
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(user)
	}
}

func TestLoginSuccessful(t *testing.T) {
	email := "user@example.com"
	f, tio, _ := newTestLoginFactory(t)

	srv := httptest.NewServer(myselfHandler(api.User{
		AccountID:    "abc123",
		DisplayName:  "Jane Doe",
		EmailAddress: &email,
		Active:       true,
	}))
	defer srv.Close()

	opts := &LoginOptions{
		Instance:   "mysite.atlassian.net",
		User:       "user@example.com",
		Token:      "tok123",
		Profile:    "default",
		Factory:    f,
		clientOpts: []api.ClientOption{api.WithBaseURL(srv.URL)},
	}

	if err := runLogin(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Logged in as Jane Doe (user@example.com) on mysite.atlassian.net") {
		t.Errorf("output = %q, want login success message", out)
	}
}

func TestLoginNullEmail(t *testing.T) {
	f, tio, _ := newTestLoginFactory(t)

	srv := httptest.NewServer(myselfHandler(api.User{
		AccountID:   "abc123",
		DisplayName: "Jane Doe",
		Active:      true,
	}))
	defer srv.Close()

	opts := &LoginOptions{
		Instance:   "mysite.atlassian.net",
		User:       "user@example.com",
		Token:      "tok123",
		Profile:    "default",
		Factory:    f,
		clientOpts: []api.ClientOption{api.WithBaseURL(srv.URL)},
	}

	if err := runLogin(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Logged in as Jane Doe on mysite.atlassian.net") {
		t.Errorf("output = %q, want message without email", out)
	}
	if strings.Contains(out, "(") {
		t.Errorf("output should not contain parenthesized email for null email: %q", out)
	}
}

func TestLogin401InvalidCredentials(t *testing.T) {
	f, _, _ := newTestLoginFactory(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"errorMessages":["Client must be authenticated"]}`))
	}))
	defer srv.Close()

	opts := &LoginOptions{
		Instance:   "mysite.atlassian.net",
		User:       "user@example.com",
		Token:      "badtoken",
		Profile:    "default",
		Factory:    f,
		clientOpts: []api.ClientOption{api.WithBaseURL(srv.URL)},
	}

	err := runLogin(opts)
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

func TestLoginMissingFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"missing instance", []string{"login", "--user", "u@e.com", "--token", "tok"}},
		{"missing user", []string{"login", "--instance", "site.net", "--token", "tok"}},
		{"missing token", []string{"login", "--instance", "site.net", "--user", "u@e.com"}},
		{"all missing", []string{"login"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, tio, _ := newTestLoginFactory(t)
			cmd := NewCmdAuth(f)
			cmd.SetOut(tio.OutBuf)
			cmd.SetErr(tio.ErrBuf)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var cliErr *clierrors.CLIError
			if !errors.As(err, &cliErr) {
				t.Fatalf("expected CLIError, got %T: %v", err, err)
			}
			if cliErr.Code != clierrors.VALIDATION_ERROR {
				t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.VALIDATION_ERROR)
			}
		})
	}
}

func TestLoginJSONOutput(t *testing.T) {
	email := "user@example.com"
	f, tio, _ := newTestLoginFactory(t)
	f.OutputJSON = true

	srv := httptest.NewServer(myselfHandler(api.User{
		AccountID:    "abc123",
		DisplayName:  "Jane Doe",
		EmailAddress: &email,
		Active:       true,
	}))
	defer srv.Close()

	opts := &LoginOptions{
		Instance:   "mysite.atlassian.net",
		User:       "user@example.com",
		Token:      "tok123",
		Profile:    "default",
		Factory:    f,
		clientOpts: []api.ClientOption{api.WithBaseURL(srv.URL)},
	}

	if err := runLogin(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if result["ok"] != true {
		t.Error("expected ok:true")
	}
	for _, field := range []string{"profile", "instance", "email", "display_name"} {
		if _, ok := result[field]; !ok {
			t.Errorf("missing JSON field %q", field)
		}
	}
	if result["email"] != email {
		t.Errorf("email = %v, want %q", result["email"], email)
	}
	if result["display_name"] != "Jane Doe" {
		t.Errorf("display_name = %v, want %q", result["display_name"], "Jane Doe")
	}
	if result["instance"] != "mysite.atlassian.net" {
		t.Errorf("instance = %v, want %q", result["instance"], "mysite.atlassian.net")
	}
	if result["profile"] != "default" {
		t.Errorf("profile = %v, want %q", result["profile"], "default")
	}
}

func TestLoginJSONNullEmail(t *testing.T) {
	f, tio, _ := newTestLoginFactory(t)
	f.OutputJSON = true

	srv := httptest.NewServer(myselfHandler(api.User{
		AccountID:   "abc123",
		DisplayName: "Jane Doe",
		Active:      true,
	}))
	defer srv.Close()

	opts := &LoginOptions{
		Instance:   "mysite.atlassian.net",
		User:       "user@example.com",
		Token:      "tok123",
		Profile:    "default",
		Factory:    f,
		clientOpts: []api.ClientOption{api.WithBaseURL(srv.URL)},
	}

	if err := runLogin(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if result["email"] != nil {
		t.Errorf("expected null email, got %v", result["email"])
	}
}

func TestLoginProfilePersistence(t *testing.T) {
	f, _, cfgPath := newTestLoginFactory(t)
	email := "user@example.com"

	srv := httptest.NewServer(myselfHandler(api.User{
		AccountID:    "abc123",
		DisplayName:  "Jane Doe",
		EmailAddress: &email,
		Active:       true,
	}))
	defer srv.Close()

	opts := &LoginOptions{
		Instance:   "mysite.atlassian.net",
		User:       "user@example.com",
		Token:      "tok123",
		Profile:    "work",
		Factory:    f,
		clientOpts: []api.ClientOption{api.WithBaseURL(srv.URL)},
	}

	if err := runLogin(opts); err != nil {
		t.Fatalf("login error: %v", err)
	}

	// Reload config from disk to verify persistence.
	cfg2, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}

	type profileGetter interface {
		GetProfile(name string) *config.Profile
		ActiveProfile() string
	}
	pg := cfg2.(profileGetter)

	profile := pg.GetProfile("work")
	if profile == nil {
		t.Fatal("profile 'work' not found after login")
	}
	if profile.Instance != "mysite.atlassian.net" {
		t.Errorf("instance = %q, want %q", profile.Instance, "mysite.atlassian.net")
	}
	if profile.User != "user@example.com" {
		t.Errorf("user = %q, want %q", profile.User, "user@example.com")
	}
	if pg.ActiveProfile() != "work" {
		t.Errorf("active profile = %q, want %q", pg.ActiveProfile(), "work")
	}

	// Verify token stored in keyring.
	token, err := keyring.Get("jira-cli", "work-token")
	if err != nil {
		t.Fatalf("retrieve token: %v", err)
	}
	if token != "tok123" {
		t.Errorf("token = %q, want %q", token, "tok123")
	}
}

func TestLoginReloginOverwrites(t *testing.T) {
	f, tio, cfgPath := newTestLoginFactory(t)
	email := "user@example.com"

	srv := httptest.NewServer(myselfHandler(api.User{
		AccountID:    "abc123",
		DisplayName:  "Jane Doe",
		EmailAddress: &email,
		Active:       true,
	}))
	defer srv.Close()

	// First login.
	opts := &LoginOptions{
		Instance:   "first.atlassian.net",
		User:       "first@example.com",
		Token:      "tok1",
		Profile:    "default",
		Factory:    f,
		clientOpts: []api.ClientOption{api.WithBaseURL(srv.URL)},
	}
	if err := runLogin(opts); err != nil {
		t.Fatalf("first login: %v", err)
	}

	tio.OutBuf.Reset()

	// Second login overwrites same profile.
	opts.Instance = "second.atlassian.net"
	opts.User = "second@example.com"
	opts.Token = "tok2"
	if err := runLogin(opts); err != nil {
		t.Fatalf("second login: %v", err)
	}

	// Reload and verify overwrite.
	cfg2, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}

	type profileGetter interface {
		GetProfile(name string) *config.Profile
	}
	pg := cfg2.(profileGetter)

	profile := pg.GetProfile("default")
	if profile == nil {
		t.Fatal("profile 'default' not found")
	}
	if profile.Instance != "second.atlassian.net" {
		t.Errorf("instance = %q, want %q", profile.Instance, "second.atlassian.net")
	}
	if profile.User != "second@example.com" {
		t.Errorf("user = %q, want %q", profile.User, "second@example.com")
	}
}

func TestLoginDefaultProfileName(t *testing.T) {
	f, _, cfgPath := newTestLoginFactory(t)
	email := "user@example.com"

	srv := httptest.NewServer(myselfHandler(api.User{
		AccountID:    "abc123",
		DisplayName:  "Jane Doe",
		EmailAddress: &email,
		Active:       true,
	}))
	defer srv.Close()

	// Profile left empty — should default to "default".
	opts := &LoginOptions{
		Instance:   "mysite.atlassian.net",
		User:       "user@example.com",
		Token:      "tok123",
		Profile:    "default",
		Factory:    f,
		clientOpts: []api.ClientOption{api.WithBaseURL(srv.URL)},
	}

	if err := runLogin(opts); err != nil {
		t.Fatalf("login error: %v", err)
	}

	cfg2, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	type profileGetter interface {
		GetProfile(name string) *config.Profile
	}
	pg := cfg2.(profileGetter)

	if pg.GetProfile("default") == nil {
		t.Fatal("profile 'default' not found")
	}
}

func TestLoginNamedProfile(t *testing.T) {
	f, _, cfgPath := newTestLoginFactory(t)
	email := "user@example.com"

	srv := httptest.NewServer(myselfHandler(api.User{
		AccountID:    "abc123",
		DisplayName:  "Jane Doe",
		EmailAddress: &email,
		Active:       true,
	}))
	defer srv.Close()

	opts := &LoginOptions{
		Instance:   "staging.atlassian.net",
		User:       "staging@example.com",
		Token:      "tok-staging",
		Profile:    "staging",
		Factory:    f,
		clientOpts: []api.ClientOption{api.WithBaseURL(srv.URL)},
	}

	if err := runLogin(opts); err != nil {
		t.Fatalf("login error: %v", err)
	}

	cfg2, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	type profileGetter interface {
		GetProfile(name string) *config.Profile
		ActiveProfile() string
	}
	pg := cfg2.(profileGetter)

	profile := pg.GetProfile("staging")
	if profile == nil {
		t.Fatal("profile 'staging' not found")
	}
	if profile.Instance != "staging.atlassian.net" {
		t.Errorf("instance = %q, want %q", profile.Instance, "staging.atlassian.net")
	}
	if pg.ActiveProfile() != "staging" {
		t.Errorf("active profile = %q, want %q", pg.ActiveProfile(), "staging")
	}
}
