package issue

import (
	"context"
	"fmt"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/adf"
	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/cmd/shared"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// CreateOptions holds all resolved inputs for the issue create command.
type CreateOptions struct {
	Factory *factory.Factory

	Project     string   // --project (or default.project config)
	Type        string   // --type
	Summary     string   // --summary
	Description string   // --description (Markdown)
	BodyFile    string   // --body-file (file path or "-" for stdin)
	Assignee    string   // --assignee
	Priority    string   // --priority
	Labels      []string // --labels
	Parent      string   // --parent
	Fields      []string // --field (repeatable, key=value)
}

// NewCmdCreate creates the "issue create" command.
func NewCmdCreate(f *factory.Factory) *cobra.Command {
	opts := &CreateOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a Jira issue",
		Long:  "Create a new Jira issue with the specified fields.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreate(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Project, "project", "p", "", "Project key (falls back to default.project config)")
	cmd.Flags().StringVarP(&opts.Type, "type", "t", "", "Issue type (e.g. Bug, Story, Task)")
	cmd.Flags().StringVarP(&opts.Summary, "summary", "s", "", "Issue summary/title")
	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Issue description (Markdown)")
	cmd.Flags().StringVar(&opts.BodyFile, "body-file", "", "Read description from file (use - for stdin)")
	cmd.Flags().StringVarP(&opts.Assignee, "assignee", "a", "", "Assignee (display name, @me, or account ID)")
	cmd.Flags().StringVar(&opts.Priority, "priority", "", "Priority (e.g. High, Medium, Low)")
	cmd.Flags().StringSliceVarP(&opts.Labels, "labels", "l", nil, "Comma-separated labels")
	cmd.Flags().StringVar(&opts.Parent, "parent", "", "Parent issue key (for subtasks)")
	cmd.Flags().StringArrayVar(&opts.Fields, "field", nil, "Custom field (key=value, repeatable)")

	return cmd
}

// runCreate executes the issue create workflow.
func runCreate(opts *CreateOptions) error {
	f := opts.Factory
	ctx := context.Background()

	// Resolve --project: flag > default.project config > error.
	project, err := resolveProject(f, opts.Project)
	if err != nil {
		return err
	}

	// Validate required flags.
	if opts.Type == "" {
		return clierrors.NewValidationError("--type is required").
			WithSuggestion("Specify an issue type, e.g. --type Bug")
	}
	if opts.Summary == "" {
		return clierrors.NewValidationError("--summary is required").
			WithSuggestion("Specify a summary, e.g. --summary 'Fix login bug'")
	}

	// Resolve description: --body-file overrides --description.
	description := opts.Description
	if opts.BodyFile != "" {
		body, err := shared.ReadBodyFile(opts.BodyFile, f.IOStreams.In)
		if err != nil {
			return err
		}
		description = body
	} else if description != "" {
		if err := shared.ValidateBodySize(description); err != nil {
			return err
		}
	}

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	// Build fields map.
	fields := map[string]interface{}{
		"project":   map[string]interface{}{"key": project},
		"issuetype": map[string]interface{}{"name": opts.Type},
		"summary":   opts.Summary,
	}

	// Description: Markdown → ADF.
	if description != "" {
		adfDoc, err := adf.Convert(description)
		if err != nil {
			return clierrors.NewValidationError(
				fmt.Sprintf("Failed to convert description to ADF: %v", err),
			)
		}
		fields["description"] = adfDoc
	}

	// Assignee resolution.
	if opts.Assignee != "" {
		accountID, err := api.ResolveUser(ctx, client, opts.Assignee)
		if err != nil {
			return err
		}
		fields["assignee"] = map[string]interface{}{"accountId": accountID}
	} else {
		// Check default.assignee config.
		assigneeDefault := configGet(f, "default.assignee")
		if assigneeDefault != "" {
			accountID, err := api.ResolveUser(ctx, client, assigneeDefault)
			if err != nil {
				return err
			}
			fields["assignee"] = map[string]interface{}{"accountId": accountID}
		}
	}

	// Priority.
	if opts.Priority != "" {
		fields["priority"] = map[string]interface{}{"name": opts.Priority}
	}

	// Labels.
	if len(opts.Labels) > 0 {
		fields["labels"] = opts.Labels
	}

	// Parent (for subtasks).
	if opts.Parent != "" {
		parentKey, err := ValidateIssueKeyOrID(opts.Parent)
		if err != nil {
			return err
		}
		fields["parent"] = map[string]interface{}{"key": parentKey}
	}

	// Custom --field key=value pairs.
	// Named flags take precedence; warn on collision.
	namedFieldKeys := map[string]bool{
		"project": true, "issuetype": true, "summary": true,
		"description": true, "assignee": true, "priority": true,
		"labels": true, "parent": true,
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
	}

	// Dry-run: validate via createmeta and output preview without creating.
	if f.DryRun {
		return runCreateDryRun(ctx, f, client, project, fields)
	}

	input := &api.CreateIssueInput{
		Fields: fields,
	}

	created, err := client.CreateIssue(ctx, input)
	if err != nil {
		return err
	}

	browseURL := client.BrowseURL(created.Key)

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	if formatter.IsJSON() {
		extras := map[string]interface{}{
			"key": created.Key,
			"id":  created.ID,
			"url": browseURL,
		}
		return formatter.OutputMutation(extras, nil)
	}

	// Text output.
	return formatter.OutputMutation(nil, func(t table.Writer) {
		fmt.Fprintf(f.IOStreams.Out, "Created %s: %s\n", created.Key, browseURL)
	})
}

// runCreateDryRun validates the create payload against live API (createmeta)
// and outputs a preview without creating the issue.
func runCreateDryRun(ctx context.Context, f *factory.Factory, client *api.Client, project string, fields map[string]interface{}) error {
	// Validate project and issue type via createmeta.
	meta, err := client.GetCreateMeta(ctx, project)
	if err != nil {
		return err
	}

	// Check that the requested issue type exists.
	requestedType := ""
	if it, ok := fields["issuetype"].(map[string]interface{}); ok {
		if name, ok := it["name"].(string); ok {
			requestedType = name
		}
	}

	typeValid := false
	var availableTypes []string
	for _, it := range meta.IssueTypes {
		availableTypes = append(availableTypes, it.Name)
		if strings.EqualFold(it.Name, requestedType) {
			typeValid = true
		}
	}

	validation := "passed (validated against live API)"
	if !typeValid && requestedType != "" {
		validation = fmt.Sprintf("warning: issue type %q not found in project %s (available: %s)",
			requestedType, project, strings.Join(availableTypes, ", "))
	}

	// Build a sanitized payload for display (replace ADF with readable summary).
	payload := make(map[string]interface{})
	for k, v := range fields {
		payload[k] = v
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if formatter.IsJSON() {
		return formatter.OutputDryRun(payload, validation, nil)
	}

	// Text output.
	return formatter.OutputDryRun(nil, "", func(tw table.Writer) {
		fmt.Fprintf(f.IOStreams.Out, "DRY RUN — issue create preview\n\n")
		tw.AppendRow(table.Row{"Project", project})
		tw.AppendRow(table.Row{"Type", requestedType})
		if s, ok := fields["summary"].(string); ok {
			tw.AppendRow(table.Row{"Summary", s})
		}
		if _, ok := fields["description"]; ok {
			tw.AppendRow(table.Row{"Description", "(set)"})
		}
		if a, ok := fields["assignee"]; ok {
			tw.AppendRow(table.Row{"Assignee", fmt.Sprintf("%v", a)})
		}
		if p, ok := fields["priority"]; ok {
			tw.AppendRow(table.Row{"Priority", fmt.Sprintf("%v", p)})
		}
		if l, ok := fields["labels"]; ok {
			tw.AppendRow(table.Row{"Labels", fmt.Sprintf("%v", l)})
		}
		if p, ok := fields["parent"]; ok {
			tw.AppendRow(table.Row{"Parent", fmt.Sprintf("%v", p)})
		}
		fmt.Fprintf(f.IOStreams.Out, "\nValidation: %s\n", validation)
	})
}

// resolveProject resolves the project key from flag, config, or returns an error.
func resolveProject(f *factory.Factory, flagProject string) (string, error) {
	if flagProject != "" {
		return flagProject, nil
	}

	// Check default.project config.
	defaultProject := configGet(f, "default.project")
	if defaultProject != "" {
		return defaultProject, nil
	}

	return "", clierrors.NewValidationError("--project is required").
		WithSuggestion("Specify --project or set a default: jira config set default.project PROJ")
}

// configGet safely retrieves a config value. Returns "" on error or missing.
func configGet(f *factory.Factory, key string) string {
	cfg, err := f.Config()
	if err != nil || cfg == nil {
		return ""
	}
	return cfg.Get(key)
}

// parseField splits a "key=value" string on the first '=' only.
// Values may contain '=' characters. Returns false if no '=' found.
func parseField(kv string) (key, value string, ok bool) {
	idx := strings.Index(kv, "=")
	if idx < 0 {
		return "", "", false
	}
	return kv[:idx], kv[idx+1:], true
}
