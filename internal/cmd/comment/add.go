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

// CommentAddOptions holds all resolved inputs for the comment add command.
type CommentAddOptions struct {
	Factory *factory.Factory

	IssueKey string // positional arg (required)
	Body     string // --body
	BodyFile string // --body-file
}

// NewCmdAdd creates the "comment add" command.
func NewCmdAdd(f *factory.Factory) *cobra.Command {
	opts := &CommentAddOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "add <issue-key>",
		Short: "Add a comment to a Jira issue",
		Long:  "Add a comment to a Jira issue. The body is Markdown, converted to ADF before sending.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := shared.ValidateIssueKeyOrID(args[0])
			if err != nil {
				return err
			}
			opts.IssueKey = key
			return runCommentAdd(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Body, "body", "", "Comment body (Markdown)")
	cmd.Flags().StringVar(&opts.BodyFile, "body-file", "", "Read comment body from file (use - for stdin)")

	return cmd
}

// runCommentAdd executes the comment add workflow.
func runCommentAdd(opts *CommentAddOptions) error {
	f := opts.Factory
	ctx := context.Background()

	// Resolve body: --body-file overrides --body.
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
			WithSuggestion("Example: jira comment add PROJ-123 --body 'My comment'")
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

	// Dry-run: validate issue exists, show preview.
	if f.DryRun {
		return runCommentAddDryRun(ctx, f, client, opts.IssueKey, adfDoc, body)
	}

	comment, err := client.AddComment(ctx, opts.IssueKey, adfDoc)
	if err != nil {
		return err
	}

	commentURL := fmt.Sprintf("https://%s/browse/%s?focusedCommentId=%s",
		client.Instance(), opts.IssueKey, comment.ID)

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	if formatter.IsJSON() {
		extras := map[string]interface{}{
			"key":        opts.IssueKey,
			"comment_id": comment.ID,
			"action":     "added",
			"url":        commentURL,
		}
		return formatter.OutputMutation(extras, nil)
	}

	// Text output.
	return formatter.OutputMutation(nil, func(tw table.Writer) {
		fmt.Fprintf(f.IOStreams.Out, "Added comment %s to %s: %s\n",
			comment.ID, opts.IssueKey, commentURL)
	})
}

// runCommentAddDryRun validates the issue exists and previews the comment payload.
func runCommentAddDryRun(ctx context.Context, f *factory.Factory, client *api.Client, issueKey string, adfDoc *adf.Node, rawBody string) error {
	// Validate issue exists by doing a minimal ListComments call.
	_, err := client.ListComments(ctx, issueKey, api.OffsetPaginationOptions{MaxResults: 1})
	if err != nil {
		return err
	}

	payload := map[string]interface{}{
		"body": adfDoc,
	}

	extras := map[string]interface{}{
		"key": issueKey,
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if formatter.IsJSON() {
		return formatter.OutputDryRunWithContext(extras, payload, "passed (issue exists)", nil)
	}

	return formatter.OutputDryRunWithContext(nil, nil, "", func(tw table.Writer) {
		fmt.Fprintf(f.IOStreams.Out, "DRY RUN — comment add preview\n\n")
		tw.AppendRow(table.Row{"Issue", issueKey})
		tw.AppendRow(table.Row{"Action", "add comment"})
		tw.AppendRow(table.Row{"Body preview", truncateBody(rawBody, 5)})
		fmt.Fprintf(f.IOStreams.Out, "\nValidation: passed (issue exists)\n")
	})
}
