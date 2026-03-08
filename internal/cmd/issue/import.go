package issue

import (
	"context"
	"fmt"
	"io"

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

// Import action constants for importResult.Action.
const (
	actionCreate  = "create"  // dry-run preview
	actionUpdate  = "update"  // dry-run preview
	actionCreated = "created" // real operation completed
	actionUpdated = "updated" // real operation completed
)

// importResult tracks the outcome of a single import operation.
type importResult struct {
	Action  string `json:"action"`             // one of the action* constants
	Key     string `json:"key"`                // real issue key
	TempKey string `json:"temp_key,omitempty"` // original temp key for creates
	URL     string `json:"url"`                // browse URL
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

	// Separate creates and updates, preserving order.
	var creates, updates []*markdown.IssueFile
	for _, issueFile := range issueFiles {
		if issueFile.IsCreate() {
			creates = append(creates, issueFile)
		} else {
			updates = append(updates, issueFile)
		}
	}

	var results []importResult

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

		fields := buildCreateFields(issueFile)

		// Description: Markdown → ADF.
		if issueFile.Description != "" {
			adfDoc, err := adf.Convert(issueFile.Description)
			if err != nil {
				return clierrors.NewValidationError(
					fmt.Sprintf("failed to convert description to ADF for %s: %v",
						issueFile.Frontmatter.Key, err),
				)
			}
			fields["description"] = adfDoc
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

		// Fetch current issue for conflict check.
		current, err := client.GetIssue(ctx, key, &api.GetIssueOptions{
			Fields: []string{"updated"},
		})
		if err != nil {
			emitPartialResults(f.IOStreams.Err, results)
			return err
		}

		// Conflict check: compare updated timestamps.
		if !opts.Force {
			if issueFile.Frontmatter.Updated == "" {
				fmt.Fprintf(f.IOStreams.Err, "Warning: no 'updated' timestamp in %s; conflict detection skipped\n", issueFile.Path)
			} else if current.Fields.Updated != "" && issueFile.Frontmatter.Updated != current.Fields.Updated {
				return clierrors.NewConflictError(
					fmt.Sprintf("conflict on %s: local updated=%s, remote updated=%s",
						key, issueFile.Frontmatter.Updated, current.Fields.Updated),
				)
			}
		}

		fields := buildUpdateFields(issueFile)

		// Description: Markdown → ADF.
		if issueFile.Description != "" {
			adfDoc, err := adf.Convert(issueFile.Description)
			if err != nil {
				return clierrors.NewValidationError(
					fmt.Sprintf("failed to convert description to ADF for %s: %v", key, err),
				)
			}
			fields["description"] = adfDoc
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
		payload := map[string]interface{}{
			"creates": len(creates),
			"updates": len(updates),
			"results": results,
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
		})
	}

	if formatter.IsJSON() {
		created := 0
		updated := 0
		for _, r := range results {
			if r.Action == actionCreated {
				created++
			} else {
				updated++
			}
		}
		extras := map[string]interface{}{
			"created": created,
			"updated": updated,
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
func buildCreateFields(issueFile *markdown.IssueFile) map[string]interface{} {
	fm := issueFile.Frontmatter
	fields := map[string]interface{}{
		"project":   map[string]interface{}{"key": fm.Project},
		"issuetype": map[string]interface{}{"name": fm.Type},
		"summary":   fm.Summary,
	}

	if fm.Priority != "" {
		fields["priority"] = map[string]interface{}{"name": fm.Priority}
	}
	if len(fm.Labels) > 0 {
		fields["labels"] = fm.Labels
	}
	if fm.Parent != "" {
		fields["parent"] = map[string]interface{}{"key": fm.Parent}
	}
	if fm.AssigneeID != "" {
		fields["assignee"] = map[string]interface{}{"accountId": fm.AssigneeID}
	}

	return fields
}

// buildUpdateFields builds the fields map for an update operation.
// Updates do NOT send type, project, parent, or status (read-only or out of MVP scope).
func buildUpdateFields(issueFile *markdown.IssueFile) map[string]interface{} {
	fm := issueFile.Frontmatter
	fields := map[string]interface{}{
		"summary": fm.Summary,
	}

	if fm.Priority != "" {
		fields["priority"] = map[string]interface{}{"name": fm.Priority}
	}
	if fm.Labels != nil {
		fields["labels"] = fm.Labels
	}
	if fm.AssigneeID != "" {
		fields["assignee"] = map[string]interface{}{"accountId": fm.AssigneeID}
	}

	return fields
}

// writeResultLine writes a single import result line to w.
func writeResultLine(w io.Writer, r importResult) {
	fmt.Fprintf(w, "  %s %s", r.Action, r.Key)
	if r.TempKey != "" {
		fmt.Fprintf(w, " (from %s)", r.TempKey)
	}
	fmt.Fprintln(w)
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
