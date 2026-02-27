package meta

import (
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/iostreams"
	"github.com/spf13/cobra"
)

func TestMetaHelp_ShowsSubcommands(t *testing.T) {
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	metaCmd := NewCmdMeta(f)

	// Wrap in a root so Execute works.
	root := &cobra.Command{Use: "jira"}
	root.AddCommand(metaCmd)
	root.SetOut(tio.OutBuf)

	root.SetArgs([]string{"meta", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()

	if !strings.Contains(out, "commands") {
		t.Error("meta help should mention 'commands' subcommand")
	}
	if !strings.Contains(out, "version") {
		t.Error("meta help should mention 'version' subcommand")
	}
}

func TestMetaCommands_NoAuthTriggeredViaParent(t *testing.T) {
	// Factory with nil client — if auth is triggered, it would panic.
	tio := iostreams.Test()
	f := factory.NewTestFactory(tio.IOStreams, nil, nil)

	metaCmd := NewCmdMeta(f)

	root := &cobra.Command{Use: "jira"}
	root.AddCommand(metaCmd)

	// Execute "meta commands" through the parent.
	root.SetArgs([]string{"meta", "commands"})
	if err := root.Execute(); err != nil {
		t.Fatalf("meta commands should work without auth via parent: %v", err)
	}
}
