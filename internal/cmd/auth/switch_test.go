package auth

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/endgameio/jira-cli/internal/config"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/iostreams"
)

// newTestSwitchFactory creates a Factory with a disk-backed config pre-populated with profiles.
func newTestSwitchFactory(t *testing.T, profiles map[string][2]string, active string) (*factory.Factory, *iostreams.TestIOStreams, string) {
	t.Helper()

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
	for name, data := range profiles {
		ps.SetProfile(name, data[0], data[1])
	}
	if active != "" {
		if err := ps.SetActiveProfile(active); err != nil {
			t.Fatalf("set active profile: %v", err)
		}
	}
	if err := ps.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}

	f := factory.NewTestFactory(tio.IOStreams, cfg, nil)
	return f, tio, cfgPath
}

func TestSwitchSuccessful(t *testing.T) {
	profiles := map[string][2]string{
		"default": {"default.atlassian.net", "default@example.com"},
		"staging": {"staging.atlassian.net", "staging@example.com"},
	}
	f, tio, cfgPath := newTestSwitchFactory(t, profiles, "default")

	opts := &SwitchOptions{
		Profile: "staging",
		Factory: f,
	}

	if err := runSwitch(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, `Switched to profile "staging"`) {
		t.Errorf("output = %q, want switch success message", out)
	}
	if !strings.Contains(out, "staging.atlassian.net") {
		t.Errorf("output = %q, want instance URL", out)
	}

	// Verify active profile changed on disk.
	cfg2, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}

	type activeProfiler interface {
		ActiveProfile() string
	}
	ap := cfg2.(activeProfiler)
	if ap.ActiveProfile() != "staging" {
		t.Errorf("active profile = %q, want %q", ap.ActiveProfile(), "staging")
	}
}

func TestSwitchUnknownProfile(t *testing.T) {
	profiles := map[string][2]string{
		"default": {"default.atlassian.net", "default@example.com"},
		"staging": {"staging.atlassian.net", "staging@example.com"},
	}
	f, _, _ := newTestSwitchFactory(t, profiles, "default")

	opts := &SwitchOptions{
		Profile: "nonexistent",
		Factory: f,
	}

	err := runSwitch(opts)
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

	// Error message should list available profiles.
	if !strings.Contains(cliErr.Message, "default") || !strings.Contains(cliErr.Message, "staging") {
		t.Errorf("message = %q, want available profiles listed", cliErr.Message)
	}
}

func TestSwitchJSONOutput(t *testing.T) {
	profiles := map[string][2]string{
		"default": {"default.atlassian.net", "default@example.com"},
		"work":    {"work.atlassian.net", "work@example.com"},
	}
	f, tio, _ := newTestSwitchFactory(t, profiles, "default")
	f.OutputJSON = true

	opts := &SwitchOptions{
		Profile: "work",
		Factory: f,
	}

	if err := runSwitch(opts); err != nil {
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
	if result["profile"] != "work" {
		t.Errorf("profile = %v, want %q", result["profile"], "work")
	}
	if result["instance"] != "work.atlassian.net" {
		t.Errorf("instance = %v, want %q", result["instance"], "work.atlassian.net")
	}
}

func TestSwitchViaCobra(t *testing.T) {
	profiles := map[string][2]string{
		"default": {"default.atlassian.net", "default@example.com"},
	}
	f, tio, _ := newTestSwitchFactory(t, profiles, "default")

	cmd := NewCmdAuth(f)
	cmd.SetOut(tio.OutBuf)
	cmd.SetErr(tio.ErrBuf)

	// No positional arg → Cobra's ExactArgs(1) should error.
	cmd.SetArgs([]string{"switch"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing profile arg, got nil")
	}
}

func TestSwitchUnknownProfileNoAvailable(t *testing.T) {
	// Edge case: no profiles at all (fresh config).
	tio := iostreams.Test()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	f := factory.NewTestFactory(tio.IOStreams, cfg, nil)

	opts := &SwitchOptions{
		Profile: "nonexistent",
		Factory: f,
	}

	err = runSwitch(opts)
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
