package auth

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/endgameio/jira-cli/internal/config"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/iostreams"
)

// newTestLogoutFactory creates a Factory with a disk-backed config and mock keyring.
// It pre-populates a profile and token so logout has something to remove.
func newTestLogoutFactory(t *testing.T, profileName string) (*factory.Factory, *iostreams.TestIOStreams, string) {
	t.Helper()
	keyring.MockInit()

	tio := iostreams.Test()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	// Set up a profile so logout can find and delete it.
	type profileSetter interface {
		SetProfile(name, instance, user string)
		SetActiveProfile(name string) error
		config.Config
	}
	ps := cfg.(profileSetter)
	ps.SetProfile(profileName, "mysite.atlassian.net", "user@example.com")
	if err := ps.SetActiveProfile(profileName); err != nil {
		t.Fatalf("set active profile: %v", err)
	}
	if err := ps.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}

	// Store a token in mock keyring.
	if err := keyring.Set("jira-cli", profileName+"-token", "tok123"); err != nil {
		t.Fatalf("store token: %v", err)
	}

	f := factory.NewTestFactory(tio.IOStreams, cfg, nil)
	return f, tio, cfgPath
}

func TestLogoutSuccessful(t *testing.T) {
	f, tio, cfgPath := newTestLogoutFactory(t, "default")

	opts := &LogoutOptions{
		Yes:     true,
		Profile: "default",
		Factory: f,
	}

	if err := runLogout(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, `Logged out from profile "default"`) {
		t.Errorf("output = %q, want logout success message", out)
	}

	// Verify profile removed from config on disk.
	cfg2, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}

	type profileGetter interface {
		GetProfile(name string) *config.Profile
		ActiveProfile() string
	}
	pg := cfg2.(profileGetter)

	if pg.GetProfile("default") != nil {
		t.Error("profile 'default' should be deleted after logout")
	}

	// Active profile should be cleared since we deleted the active one.
	// ActiveProfile() returns "default" when unset, but the profile itself shouldn't exist.
	// The config data.Active should be empty.

	// Verify token removed from keyring.
	_, err = keyring.Get("jira-cli", "default-token")
	if err != keyring.ErrNotFound {
		t.Errorf("expected token not found, got err=%v", err)
	}
}

func TestLogoutMissingYes(t *testing.T) {
	f, tio, _ := newTestLogoutFactory(t, "default")

	cmd := NewCmdAuth(f)
	cmd.SetOut(tio.OutBuf)
	cmd.SetErr(tio.ErrBuf)
	cmd.SetArgs([]string{"logout"})

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
	if !strings.Contains(cliErr.Message, "--yes") {
		t.Errorf("message = %q, want mention of --yes", cliErr.Message)
	}
}

func TestLogoutUnknownProfile(t *testing.T) {
	f, _, _ := newTestLogoutFactory(t, "default")

	opts := &LogoutOptions{
		Yes:     true,
		Profile: "nonexistent",
		Factory: f,
	}

	err := runLogout(opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.NOT_FOUND {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.NOT_FOUND)
	}
}

func TestLogoutJSONOutput(t *testing.T) {
	f, tio, _ := newTestLogoutFactory(t, "default")
	f.OutputJSON = true

	opts := &LogoutOptions{
		Yes:     true,
		Profile: "default",
		Factory: f,
	}

	if err := runLogout(opts); err != nil {
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
	if result["profile"] != "default" {
		t.Errorf("profile = %v, want %q", result["profile"], "default")
	}
	if result["action"] != "logout" {
		t.Errorf("action = %v, want %q", result["action"], "logout")
	}
}

func TestLogoutActiveProfileCleared(t *testing.T) {
	f, _, cfgPath := newTestLogoutFactory(t, "work")

	opts := &LogoutOptions{
		Yes:     true,
		Profile: "work",
		Factory: f,
	}

	if err := runLogout(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reload config and check active profile was cleared.
	cfg2, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}

	type activeProfiler interface {
		ActiveProfile() string
	}
	ap := cfg2.(activeProfiler)

	// ActiveProfile() returns "default" when unset — but "work" profile was deleted,
	// so the internal data.Active should be empty (which ActiveProfile normalizes to "default").
	if ap.ActiveProfile() != "default" {
		t.Errorf("active profile = %q, expected fallback to 'default'", ap.ActiveProfile())
	}
}

func TestLogoutNamedProfile(t *testing.T) {
	// Set up two profiles — logout one, ensure the other remains.
	keyring.MockInit()

	tio := iostreams.Test()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	type profileSetter interface {
		SetProfile(name, instance, user string)
		SetActiveProfile(name string) error
		config.Config
	}
	ps := cfg.(profileSetter)
	ps.SetProfile("default", "default.atlassian.net", "default@example.com")
	ps.SetProfile("staging", "staging.atlassian.net", "staging@example.com")
	if err := ps.SetActiveProfile("default"); err != nil {
		t.Fatalf("set active: %v", err)
	}
	if err := ps.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	keyring.Set("jira-cli", "staging-token", "tok-staging")

	f := factory.NewTestFactory(tio.IOStreams, cfg, nil)

	opts := &LogoutOptions{
		Yes:     true,
		Profile: "staging",
		Factory: f,
	}

	if err := runLogout(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reload and verify: staging gone, default still exists, active unchanged.
	cfg2, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	type profileGetter interface {
		GetProfile(name string) *config.Profile
		ActiveProfile() string
	}
	pg := cfg2.(profileGetter)

	if pg.GetProfile("staging") != nil {
		t.Error("staging profile should be deleted")
	}
	if pg.GetProfile("default") == nil {
		t.Error("default profile should still exist")
	}
	if pg.ActiveProfile() != "default" {
		t.Errorf("active profile = %q, want 'default'", pg.ActiveProfile())
	}
}
