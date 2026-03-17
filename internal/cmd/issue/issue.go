// Package issue provides the "jira issue" command group for issue management.
package issue

import (
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/spf13/cobra"
)

// NewCmdIssue creates the "issue" command group.
// Subcommands (view, create, edit, delete, move, assign, list) are registered in their own stories.
func NewCmdIssue(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue <command>",
		Short: "Manage Jira issues",
		Long:  "View, create, edit, delete, move, assign, list, export, import, and show transitions for Jira issues.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(NewCmdView(f))
	cmd.AddCommand(NewCmdCreate(f))
	cmd.AddCommand(NewCmdEdit(f))
	cmd.AddCommand(NewCmdDelete(f))
	cmd.AddCommand(NewCmdMove(f))
	cmd.AddCommand(NewCmdAssign(f))
	cmd.AddCommand(NewCmdList(f))
	cmd.AddCommand(NewCmdTransitions(f))
	cmd.AddCommand(NewCmdExport(f))
	cmd.AddCommand(NewCmdImport(f))
	cmd.AddCommand(NewCmdReconcile(f))

	return cmd
}
