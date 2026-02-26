package root

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/config"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/iostreams"
)

func TestNewCmdRoot_SubcommandRegistered(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	root := NewCmdRoot(f)

	want := []string{"auth", "issue", "search", "config", "alias"}
	for _, name := range want {
		if _, _, err := root.Find([]string{name}); err != nil {
			t.Errorf("subcommand %q not found: %v", name, err)
		}
	}
}

func TestNewCmdRoot_TypoSuggestion(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	root := NewCmdRoot(f)

	suggestions := root.SuggestionsFor("isue")
	found := false
	for _, s := range suggestions {
		if s == "issue" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'issue' in suggestions for 'isue', got %v", suggestions)
	}
}

func TestNewCmdRoot_VersionFlag(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	Version = "1.2.3"
	defer func() { Version = "dev" }()

	root := NewCmdRoot(f)
	root.SetOut(tio.OutBuf)
	root.SetArgs([]string{"--version"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if out != "jira version 1.2.3\n" {
		t.Errorf("version output = %q, want %q", out, "jira version 1.2.3\n")
	}
}

func TestNewCmdRoot_HelpNoAuth(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	root := NewCmdRoot(f)
	root.SetOut(tio.OutBuf)
	root.SetArgs([]string{"--help"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if len(out) == 0 {
		t.Error("expected help output, got empty string")
	}
}

func TestGlobalFlags_PropagateToFactory(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	root := NewCmdRoot(f)
	addTestLeafCmd(root, func() error { return nil })

	root.SetArgs([]string{
		"--profile", "staging",
		"--json",
		"--no-color",
		"--verbose",
		"--dry-run",
		"--instance", "example.atlassian.net",
		"--user", "me@example.com",
		"--token", "tok123",
		"_test-leaf",
	})

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.Profile != "staging" {
		t.Errorf("Profile = %q, want %q", f.Profile, "staging")
	}
	if !f.OutputJSON {
		t.Error("OutputJSON should be true")
	}
	if !f.NoColor {
		t.Error("NoColor should be true")
	}
	if !f.Verbose {
		t.Error("Verbose should be true")
	}
	if !f.DryRun {
		t.Error("DryRun should be true")
	}
	if f.FlagInstance != "example.atlassian.net" {
		t.Errorf("FlagInstance = %q, want %q", f.FlagInstance, "example.atlassian.net")
	}
	if f.FlagUser != "me@example.com" {
		t.Errorf("FlagUser = %q, want %q", f.FlagUser, "me@example.com")
	}
	if f.FlagToken != "tok123" {
		t.Errorf("FlagToken = %q, want %q", f.FlagToken, "tok123")
	}
}

func TestGlobalFlags_QuietShortFlag(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	root := NewCmdRoot(f)
	addTestLeafCmd(root, func() error { return nil })

	root.SetArgs([]string{"-q", "_test-leaf"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !f.Quiet {
		t.Error("Quiet should be true with -q flag")
	}
}

func TestGlobalFlags_TextFlag(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	root := NewCmdRoot(f)
	addTestLeafCmd(root, func() error { return nil })

	root.SetArgs([]string{"--text", "_test-leaf"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !f.Text {
		t.Error("Text should be true")
	}
	if f.OutputJSON {
		t.Error("OutputJSON should be false when --text is set")
	}
}

func TestGlobalFlags_JQImpliesJSON(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	root := NewCmdRoot(f)
	addTestLeafCmd(root, func() error { return nil })

	root.SetArgs([]string{"--jq", ".key", "_test-leaf"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.JQExpr != ".key" {
		t.Errorf("JQExpr = %q, want %q", f.JQExpr, ".key")
	}
	if !f.OutputJSON {
		t.Error("OutputJSON should be true when --jq is set")
	}
}

func TestGlobalFlags_JSONFromConfig(t *testing.T) {
	tio := iostreams.Test()
	cfg := writeAndLoadConfig(t, `[output]
format = "json"
`)
	f := factory.NewTestFactory(tio.IOStreams, cfg, nil)

	root := NewCmdRoot(f)
	addTestLeafCmd(root, func() error { return nil })

	root.SetArgs([]string{"_test-leaf"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !f.OutputJSON {
		t.Error("OutputJSON should be true from config output.format=json")
	}
}

func TestGlobalFlags_TextOverridesConfigJSON(t *testing.T) {
	tio := iostreams.Test()
	cfg := writeAndLoadConfig(t, `[output]
format = "json"
`)
	f := factory.NewTestFactory(tio.IOStreams, cfg, nil)

	root := NewCmdRoot(f)
	addTestLeafCmd(root, func() error { return nil })

	root.SetArgs([]string{"--text", "_test-leaf"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.OutputJSON {
		t.Error("OutputJSON should be false when --text overrides config")
	}
}

func TestGlobalFlags_NoColorDisablesColor(t *testing.T) {
	tio := iostreams.Test()
	tio.IOStreams.SetColorEnabled(true) // start with color on
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	root := NewCmdRoot(f)
	addTestLeafCmd(root, func() error { return nil })

	root.SetArgs([]string{"--no-color", "_test-leaf"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if f.IOStreams.ColorEnabled() {
		t.Error("ColorEnabled should be false after --no-color")
	}
}

func TestGlobalFlags_IsJSONSyncedToIOStreams(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	root := NewCmdRoot(f)
	addTestLeafCmd(root, func() error { return nil })

	root.SetArgs([]string{"--json", "_test-leaf"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !f.IOStreams.IsJSON {
		t.Error("IOStreams.IsJSON should be true when --json flag is set")
	}
}

func TestGlobalFlags_JSONOverridesConfig(t *testing.T) {
	tio := iostreams.Test()
	cfg := writeAndLoadConfig(t, `[output]
format = "text"
`)
	f := factory.NewTestFactory(tio.IOStreams, cfg, nil)

	root := NewCmdRoot(f)
	addTestLeafCmd(root, func() error { return nil })

	root.SetArgs([]string{"--json", "_test-leaf"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !f.OutputJSON {
		t.Error("OutputJSON should be true (--json flag overrides config)")
	}
}

func TestGlobalFlags_VerboseStoredButNoOp(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	root := NewCmdRoot(f)
	addTestLeafCmd(root, func() error { return nil })

	root.SetArgs([]string{"--verbose", "_test-leaf"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !f.Verbose {
		t.Error("Verbose should be true (stored as no-op)")
	}
}

// --- US-011b: Flag conflict tests ---

func TestFlagConflict_JSONAndText(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	root := NewCmdRoot(f)
	addTestLeafCmd(root, func() error { return nil })

	root.SetArgs([]string{"--json", "--text", "_test-leaf"})

	err := root.Execute()
	assertCLIError(t, err, clierrors.VALIDATION_ERROR, "Cannot use --json and --text together")
}

func TestFlagConflict_JQAndText(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	root := NewCmdRoot(f)
	addTestLeafCmd(root, func() error { return nil })

	root.SetArgs([]string{"--jq", ".key", "--text", "_test-leaf"})

	err := root.Execute()
	assertCLIError(t, err, clierrors.VALIDATION_ERROR, "Cannot use --jq and --text together")
}

func TestFlagConflict_QuietAndJSON(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	root := NewCmdRoot(f)
	addTestLeafCmd(root, func() error { return nil })

	root.SetArgs([]string{"--quiet", "--json", "_test-leaf"})

	err := root.Execute()
	assertCLIError(t, err, clierrors.VALIDATION_ERROR, "Cannot use --quiet with --json or --jq")
}

func TestFlagConflict_QuietAndJQ(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	root := NewCmdRoot(f)
	addTestLeafCmd(root, func() error { return nil })

	root.SetArgs([]string{"--quiet", "--jq", ".key", "_test-leaf"})

	err := root.Execute()
	assertCLIError(t, err, clierrors.VALIDATION_ERROR, "Cannot use --quiet with --json or --jq")
}

func TestFlagConflict_QuietAndDryRun_NoConflict(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	root := NewCmdRoot(f)
	addTestLeafCmd(root, func() error { return nil })

	root.SetArgs([]string{"--quiet", "--dry-run", "_test-leaf"})

	err := root.Execute()
	if err != nil {
		t.Fatalf("--quiet + --dry-run should not conflict, got: %v", err)
	}

	if !f.Quiet {
		t.Error("Quiet should be true")
	}
	if !f.DryRun {
		t.Error("DryRun should be true")
	}
}

// --- Test helpers ---

// addTestLeafCmd adds a hidden leaf command for testing PersistentPreRunE execution.
func addTestLeafCmd(root *cobra.Command, fn func() error) *cobra.Command {
	leaf := &cobra.Command{
		Use:    "_test-leaf",
		Hidden: true,
		RunE:   func(cmd *cobra.Command, args []string) error { return fn() },
	}
	root.AddCommand(leaf)
	return leaf
}

// assertCLIError checks that err is a CLIError with the expected code and message substring.
func assertCLIError(t *testing.T, err error, code clierrors.ErrorCode, wantMsg string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", code)
	}
	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != code {
		t.Errorf("error code = %s, want %s", cliErr.Code, code)
	}
	if !strings.Contains(cliErr.Message, wantMsg) {
		t.Errorf("error message = %q, want substring %q", cliErr.Message, wantMsg)
	}
}

// writeAndLoadConfig writes a TOML string to a temp file and loads it as a Config.
func writeAndLoadConfig(t *testing.T, content string) config.Config {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing test config: %v", err)
	}
	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("loading test config: %v", err)
	}
	return cfg
}
