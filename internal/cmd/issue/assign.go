package issue

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/api"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// AssignOptions holds all resolved inputs for the issue assign command.
type AssignOptions struct {
	Factory  *factory.Factory
	KeyOrID  string // positional arg: issue key
	UserArg  string // positional arg: user (display name, account ID, or @me)
	Unassign bool   // --unassign
}

// NewCmdAssign creates the "issue assign" command.
func NewCmdAssign(f *factory.Factory) *cobra.Command {
	opts := &AssignOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "assign <key-or-id> [user]",
		Short: "Assign or unassign a Jira issue",
		Long:  "Assign a Jira issue to a user by display name, account ID, or @me. Use --unassign to remove the assignee.",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := ValidateIssueKeyOrID(args[0])
			if err != nil {
				return err
			}
			opts.KeyOrID = key

			if len(args) > 1 {
				opts.UserArg = args[1]
			}

			// Validate: --unassign + user arg is a conflict.
			if opts.Unassign && opts.UserArg != "" {
				return clierrors.NewValidationError(
					"Cannot specify both --unassign and a user argument",
				).WithSuggestion("Use either --unassign or provide a user, not both")
			}

			// Must have either --unassign or a user arg.
			if !opts.Unassign && opts.UserArg == "" {
				return clierrors.NewValidationError(
					"Must provide a user argument or --unassign",
				).WithSuggestion("Run 'jira issue assign PROJ-123 \"Jane Doe\"' or 'jira issue assign PROJ-123 --unassign'")
			}

			return runAssign(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Unassign, "unassign", false, "Remove the current assignee")

	return cmd
}

// runAssign resolves the user and calls the assign API.
func runAssign(opts *AssignOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if opts.Unassign {
		return runUnassign(ctx, client, formatter, f, opts)
	}

	return runAssignUser(ctx, client, formatter, f, opts)
}

// runUnassign handles the --unassign path.
func runUnassign(ctx context.Context, client *api.Client, formatter *output.Formatter, f *factory.Factory, opts *AssignOptions) error {
	// Dry-run: validate issue exists, show preview.
	if f.DryRun {
		_, err := client.GetIssue(ctx, opts.KeyOrID, nil)
		if err != nil {
			return err
		}

		return formatter.OutputDryRun(
			map[string]interface{}{
				"key":    opts.KeyOrID,
				"action": "unassigned",
			},
			"passed",
			func(tw table.Writer) {
				tw.AppendHeader(table.Row{"FIELD", "VALUE"})
				tw.AppendRow(table.Row{"Issue", opts.KeyOrID})
				tw.AppendRow(table.Row{"Action", "Unassign"})
			},
		)
	}

	if err := client.AssignIssue(ctx, opts.KeyOrID, nil); err != nil {
		return err
	}

	if f.Quiet {
		return nil
	}

	if formatter.IsJSON() {
		return formatter.OutputMutation(map[string]interface{}{
			"key":    opts.KeyOrID,
			"action": "unassigned",
		}, nil)
	}

	fmt.Fprintf(f.IOStreams.Out, "Unassigned %s\n", opts.KeyOrID)
	return nil
}

// runAssignUser handles the assign-to-user path.
func runAssignUser(ctx context.Context, client *api.Client, formatter *output.Formatter, f *factory.Factory, opts *AssignOptions) error {
	// Resolve user to account ID.
	accountID, err := api.ResolveUser(ctx, client, opts.UserArg)
	if err != nil {
		return err
	}

	// For JSON output and dry-run, search for user details.
	var resolvedUser *api.User
	if formatter.IsJSON() || f.DryRun {
		users, searchErr := client.SearchUsers(ctx, opts.UserArg, 0, 50)
		if searchErr == nil {
			for i := range users {
				if users[i].AccountID == accountID {
					resolvedUser = &users[i]
					break
				}
			}
		}
		// If @me or direct account ID, try GetMyself as fallback.
		if resolvedUser == nil && (opts.UserArg == "@me" || accountID == opts.UserArg) {
			myself, myselfErr := client.GetMyself(ctx)
			if myselfErr == nil && myself.AccountID == accountID {
				resolvedUser = myself
			}
		}
	}

	// Dry-run: validate issue exists and user resolves, show preview.
	if f.DryRun {
		_, err := client.GetIssue(ctx, opts.KeyOrID, nil)
		if err != nil {
			return err
		}

		payload := map[string]interface{}{
			"key":       opts.KeyOrID,
			"action":    "assigned",
			"accountId": accountID,
		}
		if resolvedUser != nil {
			payload["assignee"] = resolvedUser
		}

		displayName := accountID
		if resolvedUser != nil {
			displayName = resolvedUser.DisplayName
		}

		return formatter.OutputDryRun(payload, "passed", func(tw table.Writer) {
			tw.AppendHeader(table.Row{"FIELD", "VALUE"})
			tw.AppendRow(table.Row{"Issue", opts.KeyOrID})
			tw.AppendRow(table.Row{"Action", "Assign"})
			tw.AppendRow(table.Row{"Assignee", displayName})
		})
	}

	if err := client.AssignIssue(ctx, opts.KeyOrID, &accountID); err != nil {
		return err
	}

	if f.Quiet {
		return nil
	}

	if formatter.IsJSON() {
		extras := map[string]interface{}{
			"key":       opts.KeyOrID,
			"action":    "assigned",
			"accountId": accountID,
		}
		if resolvedUser != nil {
			extras["assignee"] = resolvedUser
		}
		return formatter.OutputMutation(extras, nil)
	}

	displayName := opts.UserArg
	if resolvedUser != nil {
		displayName = resolvedUser.DisplayName
	}

	fmt.Fprintf(f.IOStreams.Out, "Assigned %s to %s\n", opts.KeyOrID, displayName)
	return nil
}
