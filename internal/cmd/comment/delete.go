package comment

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/adf"
	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/cmd/meta"
	"github.com/endgame-build/jira-cli/internal/cmd/shared"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// CommentDeleteOptions holds all resolved inputs for the comment delete command.
type CommentDeleteOptions struct {
	Factory *factory.Factory

	IssueKey  string // positional arg 1 (required)
	CommentID string // positional arg 2 (required)
	Yes       bool   // --yes/-y
}

// NewCmdDelete creates the "comment delete" command.
func NewCmdDelete(f *factory.Factory) *cobra.Command {
	opts := &CommentDeleteOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "delete <issue-key> <comment-id>",
		Short: "Delete a comment from a Jira issue",
		Long:  "Delete a comment from a Jira issue. Requires --yes to confirm deletion.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := shared.ValidateIssueKeyOrID(args[0])
			if err != nil {
				return err
			}
			opts.IssueKey = key
			cid, err := shared.ValidateCommentID(args[1])
			if err != nil {
				return err
			}
			opts.CommentID = cid
			return runCommentDelete(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "Confirm deletion")

	meta.MarkRequired(cmd, "yes")

	return cmd
}

// runCommentDelete executes the comment delete workflow.
func runCommentDelete(opts *CommentDeleteOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	// Dry-run: validate comment exists, show what would be deleted.
	if f.DryRun {
		return runCommentDeleteDryRun(ctx, f, client, opts.IssueKey, opts.CommentID)
	}

	// Require --yes for destructive action.
	if !opts.Yes {
		return clierrors.NewValidationError("Use --yes to confirm deletion").
			WithSuggestion("Example: jira comment delete PROJ-123 10042 --yes")
	}

	err = client.DeleteComment(ctx, opts.IssueKey, opts.CommentID)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	if formatter.IsJSON() {
		extras := map[string]interface{}{
			"key":        opts.IssueKey,
			"comment_id": opts.CommentID,
			"action":     "deleted",
		}
		return formatter.OutputMutation(extras, nil)
	}

	// Text output.
	fmt.Fprintf(f.IOStreams.Out, "Deleted comment %s from %s\n",
		opts.CommentID, opts.IssueKey)
	return nil
}

// runCommentDeleteDryRun validates the comment exists and previews what would be deleted.
func runCommentDeleteDryRun(ctx context.Context, f *factory.Factory, client *api.Client, issueKey, commentID string) error {
	// Validate comment (and implicitly the issue) exists.
	comment, err := client.GetComment(ctx, issueKey, commentID)
	if err != nil {
		return err
	}

	if f.Quiet {
		return nil
	}

	// Extract first line of body for preview.
	bodyPreview := ""
	if comment.Body != nil {
		bodyPreview = truncateBody(adf.ToPlaintext(comment.Body), 1)
	}

	extras := map[string]interface{}{
		"key":        issueKey,
		"comment_id": commentID,
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if formatter.IsJSON() {
		return formatter.OutputDryRunWithContext(extras, map[string]interface{}{
			"action": "delete",
		}, "passed (comment exists)", nil)
	}

	fmt.Fprintf(f.IOStreams.Out, "DRY RUN — comment delete preview\n\n")
	return formatter.OutputDryRunWithContext(nil, nil, "", func(tw table.Writer) {
		tw.AppendRow(table.Row{"Issue", issueKey})
		tw.AppendRow(table.Row{"Comment ID", commentID})
		author := "Unknown"
		if comment.Author != nil {
			author = comment.Author.DisplayName
		}
		tw.AppendRow(table.Row{"Author", author})
		tw.AppendRow(table.Row{"Created", comment.Created})
		if bodyPreview != "" {
			tw.AppendRow(table.Row{"Body", bodyPreview})
		}
		tw.AppendRow(table.Row{"Action", "delete comment"})
		tw.AppendRow(table.Row{"Validation", "passed (comment exists)"})
	})
}
