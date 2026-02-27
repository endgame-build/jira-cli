package meta

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/iostreams"
)

// newTestMetaFactory creates a factory for meta command tests.
// No API client or auth is wired — meta commands are auth-free.
func newTestMetaFactory(t *testing.T) (*factory.Factory, *iostreams.TestIOStreams) {
	t.Helper()

	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)
	return f, tio
}

// buildTestRoot constructs a minimal command tree for testing command discovery.
func buildTestRoot(f *factory.Factory) *cobra.Command {
	root := &cobra.Command{
		Use:   "jira <command> [flags]",
		Short: "Jira CLI",
	}

	// Visible subcommand group with leaf commands.
	issueCmd := &cobra.Command{
		Use:   "issue <command>",
		Short: "Manage issues",
	}
	issueViewCmd := &cobra.Command{
		Use:   "view <issue-key>",
		Short: "View an issue",
		RunE:  func(cmd *cobra.Command, args []string) error { return nil },
	}
	issueViewCmd.Flags().Bool("no-pager", false, "Disable pager")
	issueCmd.AddCommand(issueViewCmd)

	issueCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Create an issue",
		RunE:  func(cmd *cobra.Command, args []string) error { return nil },
	}
	issueCreateCmd.Flags().String("project", "", "Project key")
	issueCreateCmd.Flags().String("type", "", "Issue type")
	_ = issueCreateCmd.MarkFlagRequired("project")
	issueCmd.AddCommand(issueCreateCmd)

	root.AddCommand(issueCmd)

	// Hidden command — should be excluded.
	hiddenCmd := &cobra.Command{
		Use:    "secret-debug",
		Short:  "Debug internal state",
		Hidden: true,
		RunE:   func(cmd *cobra.Command, args []string) error { return nil },
	}
	root.AddCommand(hiddenCmd)

	// Auth-free leaf command.
	configListCmd := &cobra.Command{
		Use:   "list",
		Short: "List config values",
		RunE:  func(cmd *cobra.Command, args []string) error { return nil },
	}
	configCmd := &cobra.Command{
		Use:   "config <command>",
		Short: "Manage configuration",
	}
	configCmd.AddCommand(configListCmd)
	root.AddCommand(configCmd)

	// Meta commands command itself.
	metaCmd := &cobra.Command{
		Use:   "meta <command>",
		Short: "CLI introspection",
	}
	cmdCommands := NewCmdCommands(f)
	metaCmd.AddCommand(cmdCommands)
	root.AddCommand(metaCmd)

	return root
}

func TestMetaCommands_JSONDefault(t *testing.T) {
	f, tio := newTestMetaFactory(t)
	root := buildTestRoot(f)

	// Execute "meta commands" without --json or --text — should default to JSON.
	root.SetArgs([]string{"meta", "commands"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()

	// Must be valid JSON.
	var commands []CommandInfo
	if err := json.Unmarshal([]byte(out), &commands); err != nil {
		t.Fatalf("output should be valid JSON array: %v\nOutput: %s", err, out)
	}

	// Should include known commands.
	found := map[string]bool{}
	for _, cmd := range commands {
		found[cmd.Command] = true
	}

	if !found["jira issue view"] {
		t.Error("expected 'jira issue view' in commands")
	}
	if !found["jira issue create"] {
		t.Error("expected 'jira issue create' in commands")
	}
	if !found["jira config list"] {
		t.Error("expected 'jira config list' in commands")
	}
}

func TestMetaCommands_HiddenExcluded(t *testing.T) {
	f, tio := newTestMetaFactory(t)
	root := buildTestRoot(f)

	root.SetArgs([]string{"meta", "commands"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if strings.Contains(out, "secret-debug") {
		t.Error("hidden command 'secret-debug' should not appear in output")
	}
}

func TestMetaCommands_ArgsExtracted(t *testing.T) {
	f, tio := newTestMetaFactory(t)
	root := buildTestRoot(f)

	root.SetArgs([]string{"meta", "commands"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var commands []CommandInfo
	if err := json.Unmarshal(tio.OutBuf.Bytes(), &commands); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	// Find "jira issue view" and verify it has the <issue-key> arg.
	for _, cmd := range commands {
		if cmd.Command == "jira issue view" {
			if len(cmd.Args) != 1 {
				t.Fatalf("issue view should have 1 arg, got %d", len(cmd.Args))
			}
			if cmd.Args[0].Name != "issue-key" {
				t.Errorf("arg name = %q, want 'issue-key'", cmd.Args[0].Name)
			}
			if !cmd.Args[0].Required {
				t.Error("issue-key arg should be required (angle brackets)")
			}
			return
		}
	}
	t.Error("'jira issue view' not found in commands")
}

func TestMetaCommands_FlagsExtracted(t *testing.T) {
	f, tio := newTestMetaFactory(t)
	root := buildTestRoot(f)

	root.SetArgs([]string{"meta", "commands"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var commands []CommandInfo
	if err := json.Unmarshal(tio.OutBuf.Bytes(), &commands); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	// Find "jira issue create" and verify its flags.
	for _, cmd := range commands {
		if cmd.Command == "jira issue create" {
			foundProject := false
			for _, flag := range cmd.Flags {
				if flag.Name == "project" {
					foundProject = true
					if !flag.Required {
						t.Error("project flag should be required")
					}
					if flag.Type != "string" {
						t.Errorf("project flag type = %q, want 'string'", flag.Type)
					}
				}
			}
			if !foundProject {
				t.Error("expected 'project' flag on issue create")
			}
			return
		}
	}
	t.Error("'jira issue create' not found in commands")
}

func TestMetaCommands_TextOutput(t *testing.T) {
	f, tio := newTestMetaFactory(t)
	f.Text = true
	root := buildTestRoot(f)

	root.SetArgs([]string{"meta", "commands"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()

	// Text mode: should have table headers.
	if !strings.Contains(out, "COMMAND") {
		t.Error("text output should contain 'COMMAND' header")
	}
	if !strings.Contains(out, "DESCRIPTION") {
		t.Error("text output should contain 'DESCRIPTION' header")
	}
	if !strings.Contains(out, "FLAGS") {
		t.Error("text output should contain 'FLAGS' header")
	}

	// Should contain command entries.
	if !strings.Contains(out, "jira issue view") {
		t.Error("text output should contain 'jira issue view'")
	}

	// Should show required flags for issue create.
	if !strings.Contains(out, "--project") {
		t.Error("text output should show '--project' as required flag for issue create")
	}
}

func TestMetaCommands_NoAuthTriggered(t *testing.T) {
	// Create factory with nil client — if auth is triggered, it would panic.
	f, _ := newTestMetaFactory(t)
	root := buildTestRoot(f)

	root.SetArgs([]string{"meta", "commands"})
	// This should succeed without trying to resolve auth.
	if err := root.Execute(); err != nil {
		t.Fatalf("meta commands should work without auth: %v", err)
	}
}
