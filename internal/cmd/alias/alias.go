// Package alias provides the "jira alias" command group for command alias management.
package alias

import (
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/spf13/cobra"
)

// NewCmdAlias creates the "alias" command group.
// Subcommands (set, delete, list) are registered in their own stories.
func NewCmdAlias(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alias <command>",
		Short: "Manage command aliases",
		Long:  "Create, delete, and list command aliases.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	return cmd
}
