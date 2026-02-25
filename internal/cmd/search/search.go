// Package search provides the "jira search" command for raw JQL queries.
package search

import (
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/spf13/cobra"
)

// NewCmdSearch creates the "search" command.
// Full implementation is registered in its own story.
func NewCmdSearch(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <jql>",
		Short: "Search issues with JQL",
		Long:  "Search for Jira issues using a raw JQL query string.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	return cmd
}
