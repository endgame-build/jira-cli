package alias

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	internalConfig "github.com/endgame-build/jira-cli/internal/config"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/iostreams"
)

// newTestAliasFactory creates a Factory with a disk-backed config (no auth needed).
func newTestAliasFactory(t *testing.T) (*factory.Factory, *iostreams.TestIOStreams, string) {
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

// --- alias set ---

func TestAliasSetRoundTrip(t *testing.T) {
	f, tio, cfgPath := newTestAliasFactory(t)

	opts := &SetOptions{
		Name:    "my-issues",
		Command: "issue list --assignee @me",
		Factory: f,
	}

	if err := runAliasSet(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "my-issues") || !strings.Contains(out, "issue list --assignee @me") {
		t.Errorf("output = %q, want alias name and command", out)
	}

	// Verify persistence by reloading from disk.
	cfg2, err := internalConfig.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	mgr := cfg2.(aliasManager)
	aliases := mgr.Aliases()
	if aliases["my-issues"] != "issue list --assignee @me" {
		t.Errorf("persisted alias = %q, want %q", aliases["my-issues"], "issue list --assignee @me")
	}
}

func TestAliasSetOverwrite(t *testing.T) {
	f, _, _ := newTestAliasFactory(t)

	// Set first alias.
	opts := &SetOptions{
		Name:    "sprint",
		Command: "issue list --status 'In Progress'",
		Factory: f,
	}
	if err := runAliasSet(opts); err != nil {
		t.Fatalf("first set: %v", err)
	}

	// Overwrite with new command.
	opts.Command = "search --mine --status 'In Progress'"
	if err := runAliasSet(opts); err != nil {
		t.Fatalf("overwrite: %v", err)
	}

	cfg, _ := f.Config()
	mgr := cfg.(aliasManager)
	if mgr.Aliases()["sprint"] != "search --mine --status 'In Progress'" {
		t.Errorf("alias not overwritten: %q", mgr.Aliases()["sprint"])
	}
}

func TestAliasSetInvalidName(t *testing.T) {
	tests := []struct {
		name string
	}{
		{""},
		{"my issues"},
		{"my_issues"},
		{"my.issues"},
		{"-leading"},
		{"trailing-"},
		{"special!char"},
		{"has@sign"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _, _ := newTestAliasFactory(t)

			opts := &SetOptions{
				Name:    tt.name,
				Command: "issue list",
				Factory: f,
			}

			err := runAliasSet(opts)
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

func TestAliasSetValidNames(t *testing.T) {
	tests := []string{
		"a",
		"sprint",
		"my-issues",
		"a1",
		"test123",
		"my-long-alias-name",
	}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			f, _, _ := newTestAliasFactory(t)

			opts := &SetOptions{
				Name:    name,
				Command: "issue list",
				Factory: f,
			}

			if err := runAliasSet(opts); err != nil {
				t.Errorf("alias name %q should be valid, got error: %v", name, err)
			}
		})
	}
}

func TestAliasSetShadowDetection(t *testing.T) {
	f, _, _ := newTestAliasFactory(t)

	// Build a root command tree to test against.
	cmd := NewCmdAlias(f)
	root := cmd.Root()
	// Simulate built-in commands by adding stubs to root.
	// In real usage, root has auth, issue, search, config, alias.
	// Here we test via the Cobra integration below.

	// Test via Cobra (which has the real root with subcommands).
	_ = root // root here is just alias itself

	// Direct test: create a mock root with known subcommands.
	opts := &SetOptions{
		Name:    "auth",
		Command: "issue list",
		Factory: f,
	}

	// Build a mini root with "auth" as a subcommand.
	mockRoot := &mockRootCmd{commands: []string{"auth", "issue", "search", "config", "alias", "help", "completion"}}
	opts.rootCmd = mockRoot.asCobraRoot()

	err := runAliasSet(opts)
	if err == nil {
		t.Fatal("expected shadow error, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.VALIDATION_ERROR)
	}
	if !strings.Contains(cliErr.Message, "conflicts with built-in command") {
		t.Errorf("error message = %q, want 'conflicts with built-in command'", cliErr.Message)
	}
}

func TestAliasSetShadowAllBuiltins(t *testing.T) {
	builtins := []string{"auth", "issue", "search", "config", "alias"}

	for _, name := range builtins {
		t.Run(name, func(t *testing.T) {
			f, _, _ := newTestAliasFactory(t)

			mock := &mockRootCmd{commands: builtins}
			opts := &SetOptions{
				Name:    name,
				Command: "some command",
				Factory: f,
				rootCmd: mock.asCobraRoot(),
			}

			err := runAliasSet(opts)
			if err == nil {
				t.Fatalf("alias name %q should shadow built-in, got nil error", name)
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

func TestAliasSetJSON(t *testing.T) {
	f, tio, _ := newTestAliasFactory(t)
	f.OutputJSON = true

	opts := &SetOptions{
		Name:    "sprint",
		Command: "issue list --status 'In Progress'",
		Factory: f,
	}

	if err := runAliasSet(opts); err != nil {
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
	if result["name"] != "sprint" {
		t.Errorf("name = %v, want %q", result["name"], "sprint")
	}
	if result["command"] != "issue list --status 'In Progress'" {
		t.Errorf("command = %v, want %q", result["command"], "issue list --status 'In Progress'")
	}
}

func TestAliasSetQuiet(t *testing.T) {
	f, tio, _ := newTestAliasFactory(t)
	f.Quiet = true

	opts := &SetOptions{
		Name:    "sprint",
		Command: "issue list",
		Factory: f,
	}

	if err := runAliasSet(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tio.OutBuf.String() != "" {
		t.Errorf("expected no output in quiet mode, got %q", tio.OutBuf.String())
	}
}

// --- alias list ---

func TestAliasList(t *testing.T) {
	f, tio, _ := newTestAliasFactory(t)

	cfg, _ := f.Config()
	mgr := cfg.(aliasManager)
	mgr.SetAlias("sprint", "issue list --status 'In Progress'")
	mgr.SetAlias("mine", "issue list --assignee @me")

	opts := &ListOptions{Factory: f}

	if err := runAliasList(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "mine=issue list --assignee @me") {
		t.Errorf("output = %q, want mine alias", out)
	}
	if !strings.Contains(out, "sprint=issue list --status 'In Progress'") {
		t.Errorf("output = %q, want sprint alias", out)
	}
}

func TestAliasListEmpty(t *testing.T) {
	f, tio, _ := newTestAliasFactory(t)

	opts := &ListOptions{Factory: f}

	if err := runAliasList(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tio.OutBuf.String() != "" {
		t.Errorf("expected empty output, got %q", tio.OutBuf.String())
	}
}

func TestAliasListJSON(t *testing.T) {
	f, tio, _ := newTestAliasFactory(t)
	f.OutputJSON = true

	cfg, _ := f.Config()
	mgr := cfg.(aliasManager)
	mgr.SetAlias("sprint", "issue list")
	mgr.SetAlias("mine", "search --mine")

	opts := &ListOptions{Factory: f}

	if err := runAliasList(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", err, out)
	}

	if result["sprint"] != "issue list" {
		t.Errorf("sprint = %v, want %q", result["sprint"], "issue list")
	}
	if result["mine"] != "search --mine" {
		t.Errorf("mine = %v, want %q", result["mine"], "search --mine")
	}
}

func TestAliasListSorted(t *testing.T) {
	f, tio, _ := newTestAliasFactory(t)

	cfg, _ := f.Config()
	mgr := cfg.(aliasManager)
	mgr.SetAlias("zulu", "search z")
	mgr.SetAlias("alpha", "search a")
	mgr.SetAlias("mike", "search m")

	opts := &ListOptions{Factory: f}

	if err := runAliasList(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	if !strings.HasPrefix(lines[0], "alpha=") {
		t.Errorf("first line = %q, want alpha", lines[0])
	}
	if !strings.HasPrefix(lines[1], "mike=") {
		t.Errorf("second line = %q, want mike", lines[1])
	}
	if !strings.HasPrefix(lines[2], "zulu=") {
		t.Errorf("third line = %q, want zulu", lines[2])
	}
}

// --- no auth required ---

func TestAliasNoAuthRequired(t *testing.T) {
	tio := iostreams.Test()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	cfg, err := internalConfig.LoadFromPath(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	f := factory.NewTestFactory(tio.IOStreams, cfg, nil) // nil client

	// set
	setOpts := &SetOptions{Name: "sprint", Command: "issue list", Factory: f}
	if err := runAliasSet(setOpts); err != nil {
		t.Fatalf("alias set without auth: %v", err)
	}

	// list
	tio.OutBuf.Reset()
	listOpts := &ListOptions{Factory: f}
	if err := runAliasList(listOpts); err != nil {
		t.Fatalf("alias list without auth: %v", err)
	}
}

// --- via Cobra ---

func TestAliasViaCobra(t *testing.T) {
	f, tio, _ := newTestAliasFactory(t)

	cmd := NewCmdAlias(f)
	cmd.SetOut(tio.OutBuf)
	cmd.SetErr(tio.ErrBuf)

	// set
	cmd.SetArgs([]string{"set", "sprint", "issue list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("alias set via cobra: %v", err)
	}

	// list
	tio.OutBuf.Reset()
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("alias list via cobra: %v", err)
	}
	if !strings.Contains(tio.OutBuf.String(), "sprint=issue list") {
		t.Errorf("list output = %q, want sprint alias", tio.OutBuf.String())
	}
}

func TestAliasSetViaCobra_ShadowDetection(t *testing.T) {
	f, tio, _ := newTestAliasFactory(t)

	// Build a root command that has "auth" as a sibling (simulating the real tree).
	rootCmd := newMockRootWithAlias(f)

	rootCmd.SetOut(tio.OutBuf)
	rootCmd.SetErr(tio.ErrBuf)

	rootCmd.SetArgs([]string{"alias", "set", "auth", "some command"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected shadow error via Cobra, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.VALIDATION_ERROR {
		t.Errorf("error code = %s, want %s", cliErr.Code, clierrors.VALIDATION_ERROR)
	}
}

// --- test helpers ---

// mockRootCmd builds a cobra.Command with stub subcommands for shadow testing.
type mockRootCmd struct {
	commands []string
}

func (m *mockRootCmd) asCobraRoot() *cobra.Command {
	root := &cobra.Command{Use: "jira"}
	for _, name := range m.commands {
		root.AddCommand(&cobra.Command{
			Use:  name,
			RunE: func(cmd *cobra.Command, args []string) error { return nil },
		})
	}
	return root
}

// newMockRootWithAlias creates a root command with "auth" and "alias" subcommands.
// This simulates the real command tree for shadow detection testing via Cobra.
func newMockRootWithAlias(f *factory.Factory) *cobra.Command {
	root := &cobra.Command{
		Use:           "jira",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	root.AddCommand(&cobra.Command{
		Use:  "auth",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	})
	root.AddCommand(&cobra.Command{
		Use:  "issue",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	})
	root.AddCommand(NewCmdAlias(f))

	return root
}
