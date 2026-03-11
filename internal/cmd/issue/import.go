package issue

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/adf"
	"github.com/endgame-build/jira-cli/internal/api"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/markdown"
	"github.com/endgame-build/jira-cli/internal/output"
)

// ImportOptions holds all resolved inputs for the issue import command.
type ImportOptions struct {
	Factory *factory.Factory

	Files []string // positional args: file paths
	Dir   string   // --dir: import all .md files from directory
	Force bool     // --force: overwrite on conflict
}

// importAction is a typed constant for import result actions.
type importAction string

// Import action constants for importResult.Action.
const (
	actionCreate  importAction = "create"  // dry-run preview
	actionUpdate  importAction = "update"  // dry-run preview
	actionCreated importAction = "created" // real operation completed
	actionUpdated importAction = "updated" // real operation completed
)

// importResult tracks the outcome of a single import operation.
type importResult struct {
	Action  importAction `json:"action"`             // one of the action* constants
	Key     string       `json:"key"`                // real issue key
	TempKey string       `json:"temp_key,omitempty"` // original temp key for creates
	URL     string       `json:"url"`                // browse URL
}

// NewCmdImport creates the "issue import" command.
func NewCmdImport(f *factory.Factory) *cobra.Command {
	opts := &ImportOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "import [files...]",
		Short: "Import issues from markdown files",
		Long:  "Create or update Jira issues from local markdown files with YAML frontmatter. Files with temporary keys (PROJ-NEW-N) create new issues; files with real keys update existing ones.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && opts.Dir != "" {
				return clierrors.NewValidationError("specify files or --dir, not both").
					WithSuggestion("Use positional arguments for specific files, or --dir for a directory")
			}
			if len(args) == 0 && opts.Dir == "" {
				return clierrors.NewValidationError("specify files or --dir").
					WithSuggestion("Provide file paths as arguments, or use --dir to import from a directory")
			}
			opts.Files = args
			return runImport(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Dir, "dir", "d", "", "Import all .md files from directory (mutually exclusive with file args)")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Overwrite on conflict (skip timestamp check)")

	return cmd
}

// runImport executes the issue import workflow.
func runImport(opts *ImportOptions) error {
	f := opts.Factory
	ctx := context.Background()

	// Collect and parse files.
	var issueFiles []*markdown.IssueFile
	var err error

	if opts.Dir != "" {
		issueFiles, err = markdown.ParseDir(opts.Dir)
		if err != nil {
			return err
		}
	} else {
		for _, path := range opts.Files {
			issueFile, err := markdown.ParseFile(path)
			if err != nil {
				return err
			}
			issueFiles = append(issueFiles, issueFile)
		}
	}

	// Validate: temp-to-temp parent references.
	for _, issueFile := range issueFiles {
		if issueFile.Frontmatter.Parent != "" && markdown.IsTempKey(issueFile.Frontmatter.Parent) {
			return clierrors.NewValidationError(
				fmt.Sprintf("temp-to-temp parent reference: %s references parent %s",
					issueFile.Frontmatter.Key, issueFile.Frontmatter.Parent),
			).WithSuggestion("Create parent issues first, then update child files with the real key")
		}
	}

	// Validate creates have required fields.
	for _, issueFile := range issueFiles {
		if issueFile.IsCreate() {
			if issueFile.Frontmatter.Summary == "" {
				return clierrors.NewValidationError(
					fmt.Sprintf("missing required field 'summary' for create in %s", issueFile.Path),
				).WithSuggestion("Add 'summary: ...' to the YAML frontmatter")
			}
			if issueFile.Frontmatter.Project == "" {
				return clierrors.NewValidationError(
					fmt.Sprintf("missing required field 'project' for create in %s", issueFile.Path),
				).WithSuggestion("Add 'project: PROJ' to the YAML frontmatter")
			}
			if issueFile.Frontmatter.Type == "" {
				return clierrors.NewValidationError(
					fmt.Sprintf("missing required field 'type' for create in %s", issueFile.Path),
				).WithSuggestion("Add 'type: Task' to the YAML frontmatter")
			}
		}
	}

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	// Fetch field metadata for custom field resolution.
	customFieldMap, err := buildImportFieldMap(ctx, client)
	if err != nil {
		return err
	}

	// Validate: all custom field keys must resolve to a Jira field ID.
	for _, issueFile := range issueFiles {
		for key := range issueFile.Frontmatter.CustomFields {
			if _, ok := customFieldMap[key]; !ok {
				return clierrors.NewValidationError(
					fmt.Sprintf("unknown frontmatter key %q in %s: not a Jira field", key, issueFile.Path),
				).WithSuggestion("Check field names with 'jira schema fields' or use YAML comments (# ...) for notes")
			}
		}
	}

	// Separate creates and updates, preserving order.
	var creates, updates []*markdown.IssueFile
	for _, issueFile := range issueFiles {
		if issueFile.IsCreate() {
			creates = append(creates, issueFile)
		} else {
			updates = append(updates, issueFile)
		}
	}

	results := []importResult{}

	// Process creates first.
	for _, issueFile := range creates {
		if f.DryRun {
			results = append(results, importResult{
				Action:  actionCreate,
				Key:     "(pending)",
				TempKey: issueFile.Frontmatter.Key,
			})
			continue
		}

		fields := buildCreateFields(issueFile, customFieldMap)

		if err := setDescriptionADF(fields, issueFile.Description, issueFile.Frontmatter.Key); err != nil {
			return err
		}

		input := &api.CreateIssueInput{
			Fields: fields,
		}

		created, err := client.CreateIssue(ctx, input)
		if err != nil {
			emitPartialResults(f.IOStreams.Err, results)
			return err
		}

		results = append(results, importResult{
			Action:  actionCreated,
			Key:     created.Key,
			TempKey: issueFile.Frontmatter.Key,
			URL:     client.BrowseURL(created.Key),
		})
	}

	// Process updates.
	for _, issueFile := range updates {
		key := issueFile.Frontmatter.Key

		if f.DryRun {
			results = append(results, importResult{
				Action: actionUpdate,
				Key:    key,
			})
			continue
		}

		// Conflict check: fetch current issue and compare timestamps (skipped with --force).
		if !opts.Force {
			current, err := client.GetIssue(ctx, key, &api.GetIssueOptions{
				Fields: []string{"updated"},
			})
			if err != nil {
				emitPartialResults(f.IOStreams.Err, results)
				return err
			}

			if issueFile.Frontmatter.Updated == "" {
				fmt.Fprintf(f.IOStreams.Err, "Warning: no 'updated' timestamp in %s; conflict detection skipped\n", issueFile.Path)
			} else if current.Fields.Updated != "" && issueFile.Frontmatter.Updated != current.Fields.Updated {
				return clierrors.NewConflictError(
					fmt.Sprintf("conflict on %s: local updated=%s, remote updated=%s",
						key, issueFile.Frontmatter.Updated, current.Fields.Updated),
				)
			}
		}

		fields := buildUpdateFields(issueFile, customFieldMap)

		if err := setDescriptionADF(fields, issueFile.Description, key); err != nil {
			return err
		}

		input := &api.EditIssueInput{
			Fields: fields,
		}

		if err := client.EditIssue(ctx, key, input); err != nil {
			emitPartialResults(f.IOStreams.Err, results)
			return err
		}

		results = append(results, importResult{
			Action: actionUpdated,
			Key:    key,
			URL:    client.BrowseURL(key),
		})
	}

	// Output results.
	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	if f.DryRun {
		// Collect custom field names across all files for preview.
		customFieldNames := collectCustomFieldNames(issueFiles)

		payload := map[string]interface{}{
			"creates":       len(creates),
			"updates":       len(updates),
			"results":       results,
			"custom_fields": customFieldNames,
		}
		if formatter.IsJSON() {
			return formatter.OutputDryRun(payload, "passed", nil)
		}
		return formatter.OutputDryRun(nil, "", func(tw table.Writer) {
			fmt.Fprintf(f.IOStreams.Out, "DRY RUN — import preview\n\n")
			for _, r := range results {
				writeResultLine(f.IOStreams.Out, r)
			}
			fmt.Fprintf(f.IOStreams.Out, "\nWould process %d creates, %d updates\n", len(creates), len(updates))
			if len(customFieldNames) > 0 {
				fmt.Fprintf(f.IOStreams.Out, "Custom fields: %s\n", strings.Join(customFieldNames, ", "))
			}
		})
	}

	if formatter.IsJSON() {
		extras := map[string]interface{}{
			"created": len(creates),
			"updated": len(updates),
			"results": results,
		}
		return formatter.OutputMutation(extras, nil)
	}

	// Text output.
	return formatter.OutputMutation(nil, func(tw table.Writer) {
		for _, r := range results {
			if r.TempKey != "" {
				fmt.Fprintf(f.IOStreams.Out, "%s %s (was %s): %s\n", r.Action, r.Key, r.TempKey, r.URL)
			} else {
				fmt.Fprintf(f.IOStreams.Out, "%s %s: %s\n", r.Action, r.Key, r.URL)
			}
		}
	})
}

// buildCreateFields builds the fields map for a create operation.
func buildCreateFields(issueFile *markdown.IssueFile, customFieldMap map[string]string) map[string]interface{} {
	fm := issueFile.Frontmatter
	fields := map[string]interface{}{
		"project":   map[string]interface{}{"key": fm.Project},
		"issuetype": map[string]interface{}{"name": fm.Type},
		"summary":   fm.Summary,
	}

	setCommonFields(fields, fm)

	if fm.Parent != "" {
		fields["parent"] = map[string]interface{}{"key": fm.Parent}
	}

	injectCustomFields(fields, fm.CustomFields, customFieldMap)

	return fields
}

// buildUpdateFields builds the fields map for an update operation.
// Updates do NOT send type, project, parent, or status (read-only or out of MVP scope).
func buildUpdateFields(issueFile *markdown.IssueFile, customFieldMap map[string]string) map[string]interface{} {
	fm := issueFile.Frontmatter
	fields := map[string]interface{}{
		"summary": fm.Summary,
	}

	setCommonFields(fields, fm)

	injectCustomFields(fields, fm.CustomFields, customFieldMap)

	return fields
}

// setCommonFields sets fields shared between create and update operations.
// Labels are sent when non-nil (including empty slice, to allow clearing labels on updates).
func setCommonFields(fields map[string]interface{}, fm markdown.Frontmatter) {
	if fm.Priority != "" {
		fields["priority"] = map[string]interface{}{"name": fm.Priority}
	}
	if fm.Labels != nil {
		fields["labels"] = fm.Labels
	}
	if fm.AssigneeID != "" {
		fields["assignee"] = map[string]interface{}{"accountId": fm.AssigneeID}
	}
}

// setDescriptionADF converts a Markdown description to ADF and sets it on fields.
func setDescriptionADF(fields map[string]interface{}, description, key string) error {
	if description == "" {
		return nil
	}
	adfDoc, err := adf.Convert(description)
	if err != nil {
		return clierrors.NewValidationError(
			fmt.Sprintf("failed to convert description to ADF for %s: %v", key, err),
		)
	}
	fields["description"] = adfDoc
	return nil
}

// writeResultLine writes a single import result line to w.
func writeResultLine(w io.Writer, r importResult) {
	fmt.Fprintf(w, "  %s %s", r.Action, r.Key)
	if r.TempKey != "" {
		fmt.Fprintf(w, " (from %s)", r.TempKey)
	}
	fmt.Fprintln(w)
}

// buildImportFieldMap fetches field metadata from Jira and builds a reverse
// lookup map: normalizedName → fieldID for custom field resolution during import.
func buildImportFieldMap(ctx context.Context, client *api.Client) (map[string]string, error) {
	allFields, err := client.ListFields(ctx)
	if err != nil {
		return nil, err
	}

	fieldMap := make(map[string]string)
	for _, f := range allFields {
		norm := markdown.NormalizeFieldName(f.Name)
		if norm == "" || markdown.IsBuiltinKey(norm) {
			continue
		}
		// First field wins for duplicate normalized names.
		if _, exists := fieldMap[norm]; !exists {
			fieldMap[norm] = f.ID
		}
	}
	return fieldMap, nil
}

// injectCustomFields adds resolved custom fields to the API fields map.
// Each custom field key (normalized name) is looked up in customFieldMap to
// get the Jira field ID, and the value is sent as-is.
func injectCustomFields(fields map[string]interface{}, customFields map[string]interface{}, customFieldMap map[string]string) {
	for key, val := range customFields {
		fieldID, ok := customFieldMap[key]
		if !ok {
			continue // already validated
		}
		fields[fieldID] = val
	}
}

// collectCustomFieldNames gathers all unique custom field names across issue files.
func collectCustomFieldNames(issueFiles []*markdown.IssueFile) []string {
	seen := make(map[string]bool)
	for _, issueFile := range issueFiles {
		for key := range issueFile.Frontmatter.CustomFields {
			seen[key] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// emitPartialResults writes completed import results to stderr before an error return,
// so the user knows which operations succeeded before the failure.
func emitPartialResults(w io.Writer, results []importResult) {
	if len(results) == 0 {
		return
	}
	fmt.Fprintf(w, "\nPartial import: %d operations completed before error:\n", len(results))
	for _, r := range results {
		writeResultLine(w, r)
	}
}
