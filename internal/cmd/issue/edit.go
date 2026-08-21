package issue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/adf"
	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/cmd/shared"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// EditOptions holds all resolved inputs for the issue edit command.
type EditOptions struct {
	Factory *factory.Factory

	KeyOrID      string   // positional arg
	Summary      string   // --summary
	Description  string   // --description (Markdown)
	BodyFile     string   // --body-file
	Assignee     string   // --assignee
	Priority     string   // --priority
	Parent       string   // --parent (set-only; removal not supported)
	Labels       []string // --labels (replaces all)
	AddLabels    []string // --add-labels (delta add)
	RemoveLabels []string // --remove-labels (delta remove)
	Fields       []string // --field (repeatable, key=value)

	// Track which flags were explicitly set by the user.
	summarySet     bool
	descriptionSet bool
	assigneeSet    bool
	labelsSet      bool
	parentSet      bool
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
			opts.parentSet = cmd.Flags().Changed("parent")

			return runEdit(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Summary, "summary", "s", "", "Issue summary/title")
	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Issue description (Markdown)")
	cmd.Flags().StringVar(&opts.BodyFile, "body-file", "", "Read description from file (use - for stdin)")
	cmd.Flags().StringVarP(&opts.Assignee, "assignee", "a", "", "Assignee (display name, @me, or account ID; empty string unassigns)")
	cmd.Flags().StringVar(&opts.Priority, "priority", "", "Priority (e.g. High, Medium, Low)")
	cmd.Flags().StringVar(&opts.Parent, "parent", "", "Parent issue key (epic for stories, parent issue for subtasks)")
	cmd.Flags().StringSliceVarP(&opts.Labels, "labels", "l", nil, "Comma-separated labels (replaces all)")
	cmd.Flags().StringSliceVar(&opts.AddLabels, "add-labels", nil, "Labels to add (comma-separated)")
	cmd.Flags().StringSliceVar(&opts.RemoveLabels, "remove-labels", nil, "Labels to remove (comma-separated)")
	cmd.Flags().StringArrayVar(&opts.Fields, "field", nil, "Custom field (key=value, repeatable)")

	return cmd
}

// runEdit executes the issue edit workflow.
func runEdit(opts *EditOptions) error {
	f := opts.Factory
	ctx := context.Background()

	// Validate: --labels is mutually exclusive with --add-labels / --remove-labels.
	if opts.labelsSet && (len(opts.AddLabels) > 0 || len(opts.RemoveLabels) > 0) {
		return clierrors.NewValidationError("--labels cannot be combined with --add-labels or --remove-labels").
			WithSuggestion("Use --labels to replace all labels, or --add-labels/--remove-labels for incremental changes")
	}

	// At least one field flag must be specified.
	hasField := opts.summarySet || opts.descriptionSet || opts.BodyFile != "" ||
		opts.assigneeSet || opts.Priority != "" || opts.labelsSet || opts.parentSet ||
		len(opts.AddLabels) > 0 || len(opts.RemoveLabels) > 0 || len(opts.Fields) > 0
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

	// Parent: set-only. Removing a parent is not supported.
	if opts.parentSet {
		if opts.Parent == "" {
			return clierrors.NewValidationError("--parent cannot be empty; removing a parent is not supported").
				WithSuggestion("Provide a parent issue key, e.g. --parent PROJ-100")
		}
		parentKey, err := ValidateIssueKeyOrID(opts.Parent)
		if err != nil {
			return err
		}
		fields["parent"] = map[string]interface{}{"key": parentKey}
	}

	// Labels: replaces all labels on the issue (via fields).
	if opts.labelsSet {
		if opts.Labels == nil {
			fields["labels"] = []string{}
		} else {
			fields["labels"] = opts.Labels
		}
	}

	// Build update map for add/remove label operations.
	update := map[string]json.RawMessage{}

	if len(opts.AddLabels) > 0 || len(opts.RemoveLabels) > 0 {
		var ops []map[string]string
		for _, l := range opts.AddLabels {
			ops = append(ops, map[string]string{"add": l})
		}
		for _, l := range opts.RemoveLabels {
			ops = append(ops, map[string]string{"remove": l})
		}
		opsJSON, err := json.Marshal(ops)
		if err != nil {
			return fmt.Errorf("marshal label operations: %w", err)
		}
		update["labels"] = opsJSON
	}

	// Custom --field key=value pairs.
	// Named flags take precedence; warn on collision.
	namedFieldKeys := map[string]bool{
		"summary": true, "description": true, "assignee": true,
		"priority": true, "labels": true, "parent": true,
	}

	var updatedFields []string
	for k := range fields {
		updatedFields = append(updatedFields, k)
	}
	if len(opts.AddLabels) > 0 || len(opts.RemoveLabels) > 0 {
		updatedFields = append(updatedFields, "labels")
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

	// Dry-run: fetch current issue and show diff preview without mutating.
	if f.DryRun {
		return runEditDryRun(ctx, f, client, opts, fields, update, updatedFields)
	}

	input := &api.EditIssueInput{
		Fields: fields,
	}
	if len(update) > 0 {
		input.Update = update
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

// editChange represents a single field change for dry-run output.
type editChange struct {
	Field string      `json:"field"`
	From  interface{} `json:"from"`
	To    interface{} `json:"to"`
}

// runEditDryRun fetches the current issue, computes a before/after diff per field,
// and outputs a diff-style preview without calling the edit endpoint.
func runEditDryRun(ctx context.Context, f *factory.Factory, client *api.Client, opts *EditOptions, fields map[string]interface{}, update map[string]json.RawMessage, updatedFields []string) error {
	// Fetch current issue to compute diff.
	issue, err := client.GetIssue(ctx, opts.KeyOrID, &api.GetIssueOptions{
		Fields: []string{"summary", "description", "assignee", "priority", "labels", "parent"},
	})
	if err != nil {
		return err
	}

	var changes []editChange

	if _, ok := fields["summary"]; ok {
		changes = append(changes, editChange{
			Field: "summary",
			From:  issue.Fields.Summary,
			To:    fields["summary"],
		})
	}

	if _, ok := fields["description"]; ok {
		from := adf.ExtractText(issue.Fields.Description)
		changes = append(changes, editChange{
			Field: "description",
			From:  from,
			To:    "(updated)",
		})
	}

	if _, ok := fields["assignee"]; ok {
		var from interface{}
		if issue.Fields.Assignee != nil {
			from = issue.Fields.Assignee.DisplayName
		}
		var to interface{}
		if fields["assignee"] == nil {
			to = "(unassigned)"
		} else if m, ok := fields["assignee"].(map[string]interface{}); ok {
			to = m["accountId"]
		}
		changes = append(changes, editChange{
			Field: "assignee",
			From:  from,
			To:    to,
		})
	}

	if _, ok := fields["priority"]; ok {
		var from interface{}
		if issue.Fields.Priority != nil {
			from = issue.Fields.Priority.Name
		}
		if m, ok := fields["priority"].(map[string]interface{}); ok {
			changes = append(changes, editChange{
				Field: "priority",
				From:  from,
				To:    m["name"],
			})
		}
	}

	if _, ok := fields["labels"]; ok {
		changes = append(changes, editChange{
			Field: "labels",
			From:  issue.Fields.Labels,
			To:    fields["labels"],
		})
	}

	if _, ok := fields["parent"]; ok {
		var from interface{}
		if issue.Fields.Parent != nil {
			from = issue.Fields.Parent.Key
		}
		if m, ok := fields["parent"].(map[string]interface{}); ok {
			changes = append(changes, editChange{
				Field: "parent",
				From:  from,
				To:    m["key"],
			})
		}
	}

	// Add/remove label operations: compute resulting labels.
	if _, ok := update["labels"]; ok {
		currentLabels := issue.Fields.Labels
		resultLabels := computeLabelDelta(currentLabels, opts.AddLabels, opts.RemoveLabels)
		changes = append(changes, editChange{
			Field: "labels",
			From:  currentLabels,
			To:    resultLabels,
		})
	}

	// Custom fields.
	for _, kv := range opts.Fields {
		key, value, ok := parseField(kv)
		if !ok || namedFieldKey(key) {
			continue
		}
		var from interface{}
		if raw, ok := issue.Fields.CustomFields[key]; ok {
			from = string(raw)
		}
		changes = append(changes, editChange{
			Field: key,
			From:  from,
			To:    value,
		})
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if formatter.IsJSON() {
		return formatter.OutputDryRun(changes, "passed", nil)
	}

	// Text output: diff-style preview.
	return formatter.OutputDryRun(nil, "", func(tw table.Writer) {
		fmt.Fprintf(f.IOStreams.Out, "DRY RUN — issue edit preview for %s\n\n", opts.KeyOrID)
		tw.AppendHeader(table.Row{"Field", "From", "To"})
		for _, c := range changes {
			tw.AppendRow(table.Row{c.Field, formatDiffValue(c.From), formatDiffValue(c.To)})
		}
		fmt.Fprintf(f.IOStreams.Out, "\nValidation: passed\n")
	})
}

// computeLabelDelta applies add/remove operations to a set of labels.
func computeLabelDelta(current []string, add, remove []string) []string {
	labelSet := make(map[string]bool)
	for _, l := range current {
		labelSet[l] = true
	}
	for _, l := range add {
		labelSet[l] = true
	}
	for _, l := range remove {
		delete(labelSet, l)
	}
	result := make([]string, 0, len(labelSet))
	for l := range labelSet {
		result = append(result, l)
	}
	return result
}

// namedFieldKey returns true if the key collides with a named flag.
func namedFieldKey(key string) bool {
	switch key {
	case "summary", "description", "assignee", "priority", "labels", "parent":
		return true
	}
	return false
}

// formatDiffValue formats a value for diff display.
func formatDiffValue(v interface{}) string {
	if v == nil {
		return "(none)"
	}
	switch val := v.(type) {
	case string:
		if val == "" {
			return "(empty)"
		}
		return val
	case []string:
		if len(val) == 0 {
			return "(none)"
		}
		return strings.Join(val, ", ")
	case []interface{}:
		if len(val) == 0 {
			return "(none)"
		}
		parts := make([]string, len(val))
		for i, v := range val {
			parts[i] = fmt.Sprintf("%v", v)
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprintf("%v", v)
	}
}
