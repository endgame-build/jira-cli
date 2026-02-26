package issue

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

// EditOptions holds all resolved inputs for the issue edit command.
type EditOptions struct {
	Factory *factory.Factory

	KeyOrID     string   // positional arg
	Summary     string   // --summary
	Description string   // --description (Markdown)
	BodyFile    string   // --body-file
	Assignee    string   // --assignee
	Priority    string   // --priority
	Labels      []string // --labels
	Fields      []string // --field (repeatable, key=value)

	// Track which flags were explicitly set by the user.
	summarySet     bool
	descriptionSet bool
	assigneeSet    bool
	labelsSet      bool
}

// NewCmdEdit creates the "issue edit" command.
func NewCmdEdit(f *factory.Factory) *cobra.Command {
	opts := &EditOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "edit <key-or-id>",
		Short: "Edit a Jira issue",
		Long:  "Update fields on an existing Jira issue.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := ValidateIssueKeyOrID(args[0])
			if err != nil {
				return err
			}
			opts.KeyOrID = key

			// Track which flags were explicitly set.
			opts.summarySet = cmd.Flags().Changed("summary")
			opts.descriptionSet = cmd.Flags().Changed("description")
			opts.assigneeSet = cmd.Flags().Changed("assignee")
			opts.labelsSet = cmd.Flags().Changed("labels")

			return runEdit(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Summary, "summary", "s", "", "Issue summary/title")
	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Issue description (Markdown)")
	cmd.Flags().StringVar(&opts.BodyFile, "body-file", "", "Read description from file (use - for stdin)")
	cmd.Flags().StringVarP(&opts.Assignee, "assignee", "a", "", "Assignee (display name, @me, or account ID; empty string unassigns)")
	cmd.Flags().StringVar(&opts.Priority, "priority", "", "Priority (e.g. High, Medium, Low)")
	cmd.Flags().StringSliceVarP(&opts.Labels, "labels", "l", nil, "Comma-separated labels (replaces all)")
	cmd.Flags().StringArrayVar(&opts.Fields, "field", nil, "Custom field (key=value, repeatable)")

	return cmd
}

// runEdit executes the issue edit workflow.
func runEdit(opts *EditOptions) error {
	f := opts.Factory
	ctx := context.Background()

	// At least one field flag must be specified.
	hasField := opts.summarySet || opts.descriptionSet || opts.BodyFile != "" ||
		opts.assigneeSet || opts.Priority != "" || opts.labelsSet || len(opts.Fields) > 0
	if !hasField {
		return clierrors.NewValidationError("At least one field flag is required").
			WithSuggestion("Specify a field to update, e.g. --summary 'New title'")
	}

	// --summary '' is not allowed.
	if opts.summarySet && opts.Summary == "" {
		return clierrors.NewValidationError("Summary cannot be empty").
			WithSuggestion("Provide a non-empty summary with --summary")
	}

	// Resolve description: --body-file overrides --description.
	description := opts.Description
	hasDescription := opts.descriptionSet
	if opts.BodyFile != "" {
		body, err := shared.ReadBodyFile(opts.BodyFile, f.IOStreams.In)
		if err != nil {
			return err
		}
		description = body
		hasDescription = true
	} else if hasDescription && description != "" {
		if err := shared.ValidateBodySize(description); err != nil {
			return err
		}
	}

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	// Build fields map — only include fields that were explicitly set.
	fields := map[string]interface{}{}

	if opts.summarySet {
		fields["summary"] = opts.Summary
	}

	// Description: Markdown → ADF. Empty string clears (sends empty ADF document).
	if hasDescription {
		adfDoc, err := adf.Convert(description)
		if err != nil {
			return clierrors.NewValidationError(
				fmt.Sprintf("Failed to convert description to ADF: %v", err),
			)
		}
		fields["description"] = adfDoc
	}

	// Assignee: empty string unassigns (sends null accountId).
	if opts.assigneeSet {
		if opts.Assignee == "" {
			fields["assignee"] = nil
		} else {
			accountID, err := api.ResolveUser(ctx, client, opts.Assignee)
			if err != nil {
				return err
			}
			fields["assignee"] = map[string]interface{}{"accountId": accountID}
		}
	}

	if opts.Priority != "" {
		fields["priority"] = map[string]interface{}{"name": opts.Priority}
	}

	// Labels: replaces all labels on the issue.
	if opts.labelsSet {
		if opts.Labels == nil {
			fields["labels"] = []string{}
		} else {
			fields["labels"] = opts.Labels
		}
	}

	// Custom --field key=value pairs.
	// Named flags take precedence; warn on collision.
	namedFieldKeys := map[string]bool{
		"summary": true, "description": true, "assignee": true,
		"priority": true, "labels": true,
	}

	var updatedFields []string
	for k := range fields {
		updatedFields = append(updatedFields, k)
	}

	for _, kv := range opts.Fields {
		key, value, ok := parseField(kv)
		if !ok {
			return clierrors.NewValidationError(
				fmt.Sprintf("Invalid --field format: %q (expected key=value)", kv),
			).WithSuggestion("Use --field key=value format, e.g. --field customfield_10001=high")
		}
		if namedFieldKeys[key] {
			fmt.Fprintf(f.IOStreams.Err, "Warning: --field %q ignored (overridden by named flag --%s)\n", key, key)
			continue
		}
		fields[key] = value
		updatedFields = append(updatedFields, key)
	}

	input := &api.EditIssueInput{
		Fields: fields,
	}

	if err := client.EditIssue(ctx, opts.KeyOrID, input); err != nil {
		return err
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	if formatter.IsJSON() {
		extras := map[string]interface{}{
			"key":            opts.KeyOrID,
			"updated_fields": updatedFields,
		}
		return formatter.OutputMutation(extras, nil)
	}

	// Text output.
	return formatter.OutputMutation(nil, func(t table.Writer) {
		fmt.Fprintf(f.IOStreams.Out, "Updated %s\n", opts.KeyOrID)
	})
}
