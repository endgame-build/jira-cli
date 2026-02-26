package config

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	internalConfig "github.com/endgameio/jira-cli/internal/config"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/iostreams"
)

// newTestConfigFactory creates a Factory with a disk-backed config (no auth needed).
func newTestConfigFactory(t *testing.T) (*factory.Factory, *iostreams.TestIOStreams, string) {
	t.Helper()

	tio := iostreams.Test()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := internalConfig.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	f := factory.NewTestFactory(tio.IOStreams, cfg, nil)
	return f, tio, cfgPath
}

// --- config set ---

func TestConfigSetRoundTrip(t *testing.T) {
	f, tio, cfgPath := newTestConfigFactory(t)

	opts := &SetOptions{
		Key:     "default.project",
		Value:   "PROJ",
		Factory: f,
	}

	if err := runConfigSet(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "default.project") || !strings.Contains(out, "PROJ") {
		t.Errorf("output = %q, want key and value", out)
	}

	// Verify persistence by reloading from disk.
	cfg2, err := internalConfig.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if v := cfg2.Get("default.project"); v != "PROJ" {
		t.Errorf("persisted value = %q, want %q", v, "PROJ")
	}
}

func TestConfigSetInvalidKey(t *testing.T) {
	f, _, _ := newTestConfigFactory(t)

	opts := &SetOptions{
		Key:     "bogus.key",
		Value:   "whatever",
		Factory: f,
	}

	err := runConfigSet(opts)
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
}

func TestConfigSetInvalidValue(t *testing.T) {
	f, _, _ := newTestConfigFactory(t)

	opts := &SetOptions{
		Key:     "output.format",
		Value:   "yaml",
		Factory: f,
	}

	err := runConfigSet(opts)
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
}

func TestConfigSetJSON(t *testing.T) {
	f, tio, _ := newTestConfigFactory(t)
	f.OutputJSON = true

	opts := &SetOptions{
		Key:     "default.project",
		Value:   "PROJ",
		Factory: f,
	}

	if err := runConfigSet(opts); err != nil {
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
	if result["key"] != "default.project" {
		t.Errorf("key = %v, want %q", result["key"], "default.project")
	}
	if result["value"] != "PROJ" {
		t.Errorf("value = %v, want %q", result["value"], "PROJ")
	}
}

func TestConfigSetQuiet(t *testing.T) {
	f, tio, _ := newTestConfigFactory(t)
	f.Quiet = true

	opts := &SetOptions{
		Key:     "default.project",
		Value:   "PROJ",
		Factory: f,
	}

	if err := runConfigSet(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tio.OutBuf.String() != "" {
		t.Errorf("expected no output in quiet mode, got %q", tio.OutBuf.String())
	}
}

// --- config get ---

func TestConfigGetSet(t *testing.T) {
	f, tio, _ := newTestConfigFactory(t)

	// Set first.
	cfg, _ := f.Config()
	cfg.Set("default.project", "MYPROJ")

	opts := &GetOptions{
		Key:     "default.project",
		Factory: f,
	}

	if err := runConfigGet(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if strings.TrimSpace(out) != "MYPROJ" {
		t.Errorf("output = %q, want %q", strings.TrimSpace(out), "MYPROJ")
	}
}

func TestConfigGetUnset(t *testing.T) {
	f, tio, _ := newTestConfigFactory(t)

	opts := &GetOptions{
		Key:     "default.project",
		Factory: f,
	}

	if err := runConfigGet(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Unset key should print just a newline (empty string + newline).
	out := tio.OutBuf.String()
	if out != "\n" {
		t.Errorf("output = %q, want %q", out, "\n")
	}
}

func TestConfigGetUnknownKey(t *testing.T) {
	f, tio, _ := newTestConfigFactory(t)

	opts := &GetOptions{
		Key:     "bogus.key",
		Factory: f,
	}

	if err := runConfigGet(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Unknown keys return empty string (same as unset).
	out := tio.OutBuf.String()
	if out != "\n" {
		t.Errorf("output = %q, want %q", out, "\n")
	}
}

// --- config list ---

func TestConfigList(t *testing.T) {
	f, tio, _ := newTestConfigFactory(t)

	cfg, _ := f.Config()
	cfg.Set("default.project", "PROJ")
	cfg.Set("output.format", "json")

	opts := &ListOptions{Factory: f}

	if err := runConfigList(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "default.project=PROJ") {
		t.Errorf("output = %q, want default.project=PROJ", out)
	}
	if !strings.Contains(out, "output.format=json") {
		t.Errorf("output = %q, want output.format=json", out)
	}
}

func TestConfigListEmpty(t *testing.T) {
	f, tio, _ := newTestConfigFactory(t)

	opts := &ListOptions{Factory: f}

	if err := runConfigList(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No values set → no output.
	out := tio.OutBuf.String()
	if out != "" {
		t.Errorf("output = %q, want empty", out)
	}
}

func TestConfigListJSON(t *testing.T) {
	f, tio, _ := newTestConfigFactory(t)
	f.OutputJSON = true

	cfg, _ := f.Config()
	cfg.Set("default.project", "PROJ")
	cfg.Set("output.format", "text")

	opts := &ListOptions{Factory: f}

	if err := runConfigList(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if result["default.project"] != "PROJ" {
		t.Errorf("default.project = %v, want %q", result["default.project"], "PROJ")
	}
	if result["output.format"] != "text" {
		t.Errorf("output.format = %v, want %q", result["output.format"], "text")
	}
}

func TestConfigListSorted(t *testing.T) {
	f, tio, _ := newTestConfigFactory(t)

	cfg, _ := f.Config()
	cfg.Set("output.format", "json")
	cfg.Set("default.assignee", "me@example.com")
	cfg.Set("default.project", "PROJ")

	opts := &ListOptions{Factory: f}

	if err := runConfigList(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// Expect sorted order: default.assignee, default.project, output.format.
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "default.assignee=") {
		t.Errorf("first line = %q, want default.assignee", lines[0])
	}
	if !strings.HasPrefix(lines[1], "default.project=") {
		t.Errorf("second line = %q, want default.project", lines[1])
	}
	if !strings.HasPrefix(lines[2], "output.format=") {
		t.Errorf("third line = %q, want output.format", lines[2])
	}
}

// --- no auth required ---

func TestConfigNoAuthRequired(t *testing.T) {
	// Config commands should work with nil API client (no auth).
	tio := iostreams.Test()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := internalConfig.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	f := factory.NewTestFactory(tio.IOStreams, cfg, nil) // nil client

	// set
	setOpts := &SetOptions{Key: "default.project", Value: "PROJ", Factory: f}
	if err := runConfigSet(setOpts); err != nil {
		t.Fatalf("config set without auth: %v", err)
	}

	// get
	tio.OutBuf.Reset()
	getOpts := &GetOptions{Key: "default.project", Factory: f}
	if err := runConfigGet(getOpts); err != nil {
		t.Fatalf("config get without auth: %v", err)
	}

	// list
	tio.OutBuf.Reset()
	listOpts := &ListOptions{Factory: f}
	if err := runConfigList(listOpts); err != nil {
		t.Fatalf("config list without auth: %v", err)
	}
}

// --- via Cobra ---

func TestConfigViaCobra(t *testing.T) {
	f, tio, _ := newTestConfigFactory(t)

	cmd := NewCmdConfig(f)
	cmd.SetOut(tio.OutBuf)
	cmd.SetErr(tio.ErrBuf)

	// set
	cmd.SetArgs([]string{"set", "default.project", "PROJ"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config set via cobra: %v", err)
	}

	// get
	tio.OutBuf.Reset()
	cmd.SetArgs([]string{"get", "default.project"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config get via cobra: %v", err)
	}
	if !strings.Contains(tio.OutBuf.String(), "PROJ") {
		t.Errorf("get output = %q, want PROJ", tio.OutBuf.String())
	}

	// list
	tio.OutBuf.Reset()
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config list via cobra: %v", err)
	}
	if !strings.Contains(tio.OutBuf.String(), "default.project=PROJ") {
		t.Errorf("list output = %q, want default.project=PROJ", tio.OutBuf.String())
	}
}
