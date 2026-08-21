package agent

import (
	"context"
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/adf"
	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/cmd/shared"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// DiscoverOptions holds all resolved inputs for the agent discover command.
type DiscoverOptions struct {
	Factory     *factory.Factory
	ParentKey   string
	Title       string
	Description string
	Type        string
	Priority    string
	Labels      []string
	AsSubtask   bool
	LinkType    string
	BodyFile    string
}

// NewCmdDiscover creates the "agent discover" command.
func NewCmdDiscover(f *factory.Factory) *cobra.Command {
	opts := &DiscoverOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "discover <parent-key>",
		Short: "Create a new issue linked to the current work item",
		Long:  "Create a discovered issue as a sub-task or linked issue. Inherits project, priority, and labels from the parent.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, err := shared.ValidateIssueKeyOrID(args[0])
			if err != nil {
				return err
			}
			opts.ParentKey = key

			if opts.Title == "" {
				return clierrors.NewValidationError("--title is required").
					WithSuggestion("Specify a summary for the discovered issue")
			}

			return runDiscover(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "s", "", "Summary of discovered issue (required)")
	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Description (Markdown)")
	cmd.Flags().StringVarP(&opts.Type, "type", "t", "", "Issue type (default: Sub-task or Task)")
	cmd.Flags().StringVar(&opts.Priority, "priority", "", "Priority (default: inherit from parent)")
	cmd.Flags().StringSliceVarP(&opts.Labels, "label", "l", nil, "Labels (default: inherit from parent + 'discovered')")
	cmd.Flags().BoolVar(&opts.AsSubtask, "as-subtask", true, "Create as sub-task (default true; false creates linked issue)")
	cmd.Flags().StringVar(&opts.LinkType, "link-type", "Relates", "Link type when not a sub-task")
	cmd.Flags().StringVar(&opts.BodyFile, "body-file", "", "Read description from file (use - for stdin)")

	return cmd
}

// runDiscover creates a discovered issue linked to the parent.
func runDiscover(opts *DiscoverOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	// Fetch parent issue to inherit defaults.
	parent, err := client.GetIssue(ctx, opts.ParentKey, &api.GetIssueOptions{
		Fields: []string{"project", "priority", "labels", "issuetype"},
	})
	if err != nil {
		return err
	}

	if parent.Fields.Project == nil {
		return clierrors.NewValidationError("Parent issue has no project").
			WithContext(map[string]interface{}{"parent": opts.ParentKey})
	}

	projectKey := parent.Fields.Project.Key

	// Determine if sub-task or linked issue.
	// If parent is already a sub-task, cannot create sub-sub-task.
	isSubtask := opts.AsSubtask
	if parent.Fields.IssueType != nil && parent.Fields.IssueType.Subtask {
		isSubtask = false
	}

	// Resolve issue type. The project's own name for its sub-task type varies
	// ("Sub-task" vs "Subtask"), so ask the project rather than guessing.
	issueType := opts.Type
	if issueType == "" {
		fallback := "Task"
		if isSubtask {
			fallback = "Sub-task"
		}
		issueType = ResolveIssueTypeName(ctx, client, projectKey, isSubtask, fallback)
	}

	// Resolve priority (inherit from parent if not overridden).
	priorityName := opts.Priority
	if priorityName == "" && parent.Fields.Priority != nil {
		priorityName = parent.Fields.Priority.Name
	}

	// Resolve labels (inherit from parent + add "discovered").
	labels := opts.Labels
	if labels == nil {
		labels = parent.Fields.Labels
	}
	labels = appendUnique(labels, "discovered")

	// Resolve description.
	description := opts.Description
	if opts.BodyFile != "" {
		body, err := shared.ReadBodyFile(opts.BodyFile, f.IOStreams.In)
		if err != nil {
			return err
		}
		description = body
	}

	// Build fields.
	fields := map[string]interface{}{
		"project":   map[string]interface{}{"key": projectKey},
		"issuetype": map[string]interface{}{"name": issueType},
		"summary":   opts.Title,
		"labels":    labels,
	}

	if isSubtask {
		fields["parent"] = map[string]interface{}{"key": opts.ParentKey}
	}

	if priorityName != "" {
		fields["priority"] = map[string]interface{}{"name": priorityName}
	}

	if description != "" {
		adfDoc, err := adf.Convert(description)
		if err != nil {
			return clierrors.NewValidationError(
				fmt.Sprintf("Failed to convert description to ADF: %v", err),
			)
		}
		fields["description"] = adfDoc
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	// Dry-run.
	if f.DryRun {
		relationship := "subtask"
		if !isSubtask {
			relationship = "linked (" + opts.LinkType + ")"
		}
		payload := map[string]interface{}{
			"parent":       opts.ParentKey,
			"summary":      opts.Title,
			"type":         issueType,
			"priority":     priorityName,
			"labels":       labels,
			"relationship": relationship,
		}
		return formatter.OutputDryRun(payload, "passed", func(tw table.Writer) {
			tw.AppendHeader(table.Row{"FIELD", "VALUE"})
			tw.AppendRow(table.Row{"Parent", opts.ParentKey})
			tw.AppendRow(table.Row{"Summary", opts.Title})
			tw.AppendRow(table.Row{"Type", issueType})
			tw.AppendRow(table.Row{"Priority", priorityName})
			tw.AppendRow(table.Row{"Relationship", relationship})
		})
	}

	// Create the issue.
	created, err := client.CreateIssue(ctx, &api.CreateIssueInput{Fields: fields})
	if err != nil {
		return err
	}

	// If not sub-task, create issue link.
	linkFailed := false
	if !isSubtask {
		if err := client.CreateIssueLink(ctx, &api.CreateIssueLinkInput{
			Type:         api.IssueLinkTypeRef{Name: opts.LinkType},
			InwardIssue:  api.LinkedIssueRef{Key: opts.ParentKey},
			OutwardIssue: api.LinkedIssueRef{Key: created.Key},
		}); err != nil {
			linkFailed = true
			fmt.Fprintf(f.IOStreams.Err, "Warning: issue created but link failed: %v\n", err)
		}
	}

	// Add discovery comment to parent.
	commentMD := fmt.Sprintf("Discovered %s: %s", created.Key, opts.Title)
	adfDoc, err := adf.Convert(commentMD)
	if err != nil {
		fmt.Fprintf(f.IOStreams.Err, "Warning: failed to convert discovery comment to ADF: %v\n", err)
	} else {
		if _, err := client.AddComment(ctx, opts.ParentKey, adfDoc); err != nil {
			fmt.Fprintf(f.IOStreams.Err, "Warning: failed to add discovery comment to parent: %v\n", err)
		}
	}

	if f.Quiet {
		return nil
	}

	relationship := "subtask"
	if !isSubtask {
		relationship = "linked"
	}

	if formatter.IsJSON() {
		extras := map[string]interface{}{
			"key":          created.Key,
			"parent":       opts.ParentKey,
			"relationship": relationship,
			"summary":      opts.Title,
			"type":         issueType,
			"priority":     priorityName,
		}
		// The link is best-effort, so the issue can exist without it. Say so
		// in the payload: relationship alone would assert a link that is not
		// there, and a caller reading stdout never sees the stderr warning.
		if linkFailed {
			extras["link_failed"] = true
		}
		return formatter.OutputMutation(extras, nil)
	}

	fmt.Fprintf(f.IOStreams.Out, "Discovered %s (%s of %s): %s\n", created.Key, relationship, opts.ParentKey, opts.Title)
	return nil
}

// appendUnique appends value to slice if not already present.
func appendUnique(slice []string, value string) []string {
	for _, s := range slice {
		if s == value {
			return slice
		}
	}
	return append(slice, value)
}
