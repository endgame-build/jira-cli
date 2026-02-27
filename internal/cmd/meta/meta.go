// Package meta provides the "jira meta" command group for CLI introspection.
package meta

import (
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/spf13/cobra"
)

// NewCmdMeta creates the "meta" command group.
func NewCmdMeta(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "meta <command>",
		Short: "CLI introspection and metadata",
		Long:  "Discover CLI commands, flags, and version information. Auth-free — works without authentication.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(NewCmdCommands(f))
	cmd.AddCommand(NewCmdVersion(f))

	return cmd
}
