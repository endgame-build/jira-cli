// Package auth provides the "jira auth" command group for authentication management.
package auth

import (
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/spf13/cobra"
)

// NewCmdAuth creates the "auth" command group.
// Subcommands (login, logout, status, switch) are registered in their own stories.
func NewCmdAuth(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth <command>",
		Short: "Manage authentication",
		Long:  "Login, logout, check status, and switch between Jira profiles.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	return cmd
}
