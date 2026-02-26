package issue

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// DeleteOptions holds all resolved inputs for the issue delete command.
type DeleteOptions struct {
	Factory *factory.Factory
	KeyOrID string // positional arg: issue key
	Yes     bool   // --yes/-y confirmation flag
}

// NewCmdDelete creates the "issue delete" command.
func NewCmdDelete(f *factory.Factory) *cobra.Command {
	opts := &DeleteOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "delete <key-or-id>",
		Short: "Delete a Jira issue",
		Long:  "Delete a Jira issue and its subtasks. Requires --yes to confirm.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := ValidateIssueKeyOrID(args[0])
			if err != nil {
				return err
			}
			opts.KeyOrID = key

			// --dry-run bypasses the --yes confirmation requirement.
			if !opts.Yes && !opts.Factory.DryRun {
				return clierrors.NewValidationError(
					"Use --yes to confirm deletion",
				).WithSuggestion(fmt.Sprintf("Run 'jira issue delete %s --yes' to confirm", opts.KeyOrID))
			}

			return runDelete(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "Confirm deletion")

	return cmd
}

// runDelete validates and deletes the issue.
func runDelete(opts *DeleteOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	// Dry-run: validate issue exists and permission, show preview.
	if f.DryRun {
		_, err := client.GetIssue(ctx, opts.KeyOrID, nil)
		if err != nil {
			return err
		}

		return formatter.OutputDryRun(
			map[string]interface{}{
				"key":            opts.KeyOrID,
				"action":         "deleted",
				"deleteSubtasks": true,
			},
			"passed",
			func(tw table.Writer) {
				tw.AppendHeader(table.Row{"FIELD", "VALUE"})
				tw.AppendRow(table.Row{"Issue", opts.KeyOrID})
				tw.AppendRow(table.Row{"Action", "Delete"})
				tw.AppendRow(table.Row{"Delete Subtasks", "Yes"})
			},
		)
	}

	if err := client.DeleteIssue(ctx, opts.KeyOrID, true); err != nil {
		return err
	}

	if f.Quiet {
		return nil
	}

	if formatter.IsJSON() {
		return formatter.OutputMutation(map[string]interface{}{
			"key":    opts.KeyOrID,
			"action": "deleted",
		}, nil)
	}

	fmt.Fprintf(f.IOStreams.Out, "Deleted %s\n", opts.KeyOrID)
	return nil
}
