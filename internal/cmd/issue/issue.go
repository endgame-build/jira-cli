// Package issue provides the "jira issue" command group for issue management.
package issue

import (
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/spf13/cobra"
)

// NewCmdIssue creates the "issue" command group.
// Subcommands (view, create, edit, delete, move, assign, list) are registered in their own stories.
func NewCmdIssue(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue <command>",
		Short: "Manage Jira issues",
		Long:  "View, create, edit, delete, move, assign, and list Jira issues.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(NewCmdView(f))
	cmd.AddCommand(NewCmdCreate(f))

	return cmd
}
