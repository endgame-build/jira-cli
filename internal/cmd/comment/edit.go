package comment

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/adf"
	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/cmd/shared"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// CommentEditOptions holds all resolved inputs for the comment edit command.
type CommentEditOptions struct {
	Factory *factory.Factory

	IssueKey  string // positional arg 1 (required)
	CommentID string // positional arg 2 (required)
	Body      string // --body
	BodyFile  string // --body-file
}

// NewCmdEdit creates the "comment edit" command.
func NewCmdEdit(f *factory.Factory) *cobra.Command {
	opts := &CommentEditOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "edit <issue-key> <comment-id>",
		Short: "Edit a comment on a Jira issue",
		Long:  "Edit a comment on a Jira issue. The body is Markdown, converted to ADF before sending.",
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
			return runCommentEdit(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Body, "body", "", "Comment body (Markdown)")
	cmd.Flags().StringVar(&opts.BodyFile, "body-file", "", "Read comment body from file (use - for stdin)")
	cmd.MarkFlagsMutuallyExclusive("body", "body-file")

	return cmd
}

// runCommentEdit executes the comment edit workflow.
func runCommentEdit(opts *CommentEditOptions) error {
	f := opts.Factory
	ctx := context.Background()

	// Resolve body from --body or --body-file (mutually exclusive via Cobra).
	body := opts.Body
	if opts.BodyFile != "" {
		content, err := shared.ReadBodyFile(opts.BodyFile, f.IOStreams.In)
		if err != nil {
			return err
		}
		body = content
	} else if body != "" {
		if err := shared.ValidateBodySize(body); err != nil {
			return err
		}
	}

	if body == "" {
		return clierrors.NewValidationError("Provide --body or --body-file").
			WithSuggestion("Example: jira comment edit PROJ-123 10042 --body 'Updated text'")
	}

	// Convert Markdown → ADF.
	adfDoc, err := adf.Convert(body)
	if err != nil {
		return clierrors.NewValidationError(
			fmt.Sprintf("Failed to convert body to ADF: %v", err),
		)
	}

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	// Dry-run: validate issue and comment exist, show preview.
	if f.DryRun {
		return runCommentEditDryRun(ctx, f, client, opts.IssueKey, opts.CommentID, adfDoc, body)
	}

	_, err = client.UpdateComment(ctx, opts.IssueKey, opts.CommentID, adfDoc)
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
			"action":     "updated",
		}
		return formatter.OutputMutation(extras, nil)
	}

	// Text output.
	fmt.Fprintf(f.IOStreams.Out, "Updated comment %s on %s\n",
		opts.CommentID, opts.IssueKey)
	return nil
}

// runCommentEditDryRun validates the issue and comment exist and previews the edit payload.
func runCommentEditDryRun(ctx context.Context, f *factory.Factory, client *api.Client, issueKey, commentID string, adfDoc *adf.Node, rawBody string) error {
	// Validate comment (and implicitly the issue) exists.
	_, err := client.GetComment(ctx, issueKey, commentID)
	if err != nil {
		return err
	}

	if f.Quiet {
		return nil
	}

	payload := map[string]interface{}{
		"body": adfDoc,
	}

	extras := map[string]interface{}{
		"key":        issueKey,
		"comment_id": commentID,
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if formatter.IsJSON() {
		return formatter.OutputDryRunWithContext(extras, payload, "passed (comment exists)", nil)
	}

	fmt.Fprintf(f.IOStreams.Out, "DRY RUN — comment edit preview\n\n")
	return formatter.OutputDryRunWithContext(nil, nil, "", func(tw table.Writer) {
		tw.AppendRow(table.Row{"Issue", issueKey})
		tw.AppendRow(table.Row{"Comment ID", commentID})
		tw.AppendRow(table.Row{"Action", "edit comment"})
		tw.AppendRow(table.Row{"Body preview", truncateBody(rawBody, 5)})
		tw.AppendRow(table.Row{"Validation", "passed (comment exists)"})
	})
}
