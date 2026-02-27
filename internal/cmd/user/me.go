package user

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// UserMeOptions holds all resolved inputs for the user me command.
type UserMeOptions struct {
	Factory *factory.Factory
}

// NewCmdMe creates the "user me" command.
func NewCmdMe(f *factory.Factory) *cobra.Command {
	opts := &UserMeOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "me",
		Short: "Show authenticated user info",
		Long:  "Display the currently authenticated Jira user's account ID, display name, and email.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUserMe(opts)
		},
	}

	return cmd
}

// runUserMe fetches the authenticated user and renders the output.
func runUserMe(opts *UserMeOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	user, err := client.GetMyself(ctx)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	// JSON mode: bare object with Jira API camelCase field names.
	if formatter.IsJSON() {
		return formatter.RawJSON(user)
	}

	// Text mode: key-value display.
	fmt.Fprintf(f.IOStreams.Out, "Account ID:    %s\n", user.AccountID)
	fmt.Fprintf(f.IOStreams.Out, "Display Name:  %s\n", user.DisplayName)

	email := "(hidden)"
	if user.EmailAddress != nil {
		email = *user.EmailAddress
	}
	fmt.Fprintf(f.IOStreams.Out, "Email:         %s\n", email)

	return nil
}
