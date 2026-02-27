package user

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// UserSearchOptions holds all resolved inputs for the user search command.
type UserSearchOptions struct {
	Factory *factory.Factory

	Query string // positional arg (required)
	Limit int    // --limit
}

// NewCmdSearch creates the "user search" command.
func NewCmdSearch(f *factory.Factory) *cobra.Command {
	opts := &UserSearchOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for Jira users",
		Long:  "Search for Jira users by display name or email address.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Query = args[0]
			return runUserSearch(opts)
		},
	}

	cmd.Flags().IntVar(&opts.Limit, "limit", 10, "Maximum number of users to return")

	return cmd
}

// runUserSearch executes the user search workflow.
func runUserSearch(opts *UserSearchOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	users, err := client.SearchUsers(ctx, opts.Query, 0, opts.Limit)
	if err != nil {
		return err
	}

	// Build pagination meta: total is nil (raw array), has_next_page inferred.
	hasNext := len(users) == opts.Limit
	meta := &api.PaginationMeta{
		Offset:      0,
		Limit:       opts.Limit,
		Total:       nil,
		HasNextPage: hasNext,
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	// JSON mode: list envelope with null total.
	if formatter.IsJSON() {
		return formatter.OutputList(users, meta, nil)
	}

	// Text mode: table output.
	if len(users) == 0 {
		fmt.Fprintf(f.IOStreams.Out, "No users matching %q\n", opts.Query)
		return nil
	}

	return formatter.OutputList(users, meta, func(tw table.Writer) {
		tw.AppendHeader(table.Row{"ACCOUNT ID", "DISPLAY NAME", "EMAIL", "ACTIVE"})

		for _, u := range users {
			email := "(hidden)"
			if u.EmailAddress != nil {
				email = *u.EmailAddress
			}

			active := "yes"
			if !u.Active {
				active = "no"
			}

			tw.AppendRow(table.Row{u.AccountID, u.DisplayName, email, active})
		}
	})
}
