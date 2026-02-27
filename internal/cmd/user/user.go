// Package user provides the "jira user" command group for user operations.
package user

import (
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/spf13/cobra"
)

// NewCmdUser creates the "user" command group.
func NewCmdUser(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user <command>",
		Short: "Manage users",
		Long:  "Search for users and view your authenticated user information.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(NewCmdSearch(f))
	cmd.AddCommand(NewCmdMe(f))

	return cmd
}
