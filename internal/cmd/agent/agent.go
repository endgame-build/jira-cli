// Package agent provides the "jira agent" command group for agentic SDLC.
// Commands implement a ready → claim → work → discover → close loop
// backed by Jira Cloud issues, links, and transitions.
package agent

import (
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/spf13/cobra"
)

// NewCmdAgent creates the "agent" command group.
func NewCmdAgent(f *factory.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent <command>",
		Short: "Agentic SDLC commands",
		Long:  "Execute an agentic software development lifecycle: ready → claim → work → discover → close.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(NewCmdReady(f))
	cmd.AddCommand(NewCmdClaim(f))
	cmd.AddCommand(NewCmdClose(f))
	cmd.AddCommand(NewCmdDiscover(f))
	cmd.AddCommand(NewCmdPrime(f))
	cmd.AddCommand(NewCmdStatus(f))
	cmd.AddCommand(NewCmdBlocked(f))

	return cmd
}
