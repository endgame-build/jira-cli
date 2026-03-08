package issue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/api"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/markdown"
	"github.com/endgame-build/jira-cli/internal/output"
)

// ExportOptions holds all resolved inputs for the issue export command.
type ExportOptions struct {
	Factory *factory.Factory

	Project   string // --project (falls back to default.project)
	JQL       string // --jql (overrides --project)
	OutputDir string // --output-dir (default ".")
	Limit     int    // --limit (0 = all)
}

// exportFields are the issue fields requested from Jira for export.
var exportFields = []string{
	"summary", "description", "status", "issuetype", "priority",
	"labels", "parent", "assignee", "reporter", "project", "created", "updated",
}

// NewCmdExport creates the "issue export" command.
func NewCmdExport(f *factory.Factory) *cobra.Command {
	opts := &ExportOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export Jira issues to markdown files",
		Long:  "Export Jira issues to local markdown files with YAML frontmatter. Each issue becomes a separate file organized by project.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Project, "project", "p", "", "Project key filter (falls back to default.project config)")
	cmd.Flags().StringVar(&opts.JQL, "jql", "", "Raw JQL query (overrides --project)")
	cmd.Flags().StringVarP(&opts.OutputDir, "output-dir", "o", ".", "Root output directory")
	cmd.Flags().IntVar(&opts.Limit, "limit", 0, "Maximum issues to export (0 = all)")

	return cmd
}

// runExport executes the issue export workflow.
func runExport(opts *ExportOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	// Build JQL: --jql overrides --project.
	jql, err := buildExportJQL(f, opts)
	if err != nil {
		return err
	}

	// Iterate pages, writing files as we go.
	var exported int
	var files []string
	token := ""

	for {
		pageSize := 50
		if opts.Limit > 0 {
			remaining := opts.Limit - exported
			if remaining <= 0 {
				break
			}
			if remaining < pageSize {
				pageSize = remaining
			}
		}

		results, err := client.SearchIssues(ctx, &api.SearchOptions{
			JQL:           jql,
			Fields:        exportFields,
			MaxResults:    pageSize,
			NextPageToken: token,
		})
		if err != nil {
			return err
		}

		for _, issue := range results.Issues {
			if opts.Limit > 0 && exported >= opts.Limit {
				break
			}

			relPath := markdown.IssuePath(issue)
			fullPath := filepath.Join(opts.OutputDir, relPath)

			if !f.DryRun {
				if err := writeFileAtomic(fullPath, issue); err != nil {
					return err
				}
			}

			files = append(files, relPath)
			exported++

			if exported%50 == 0 {
				fmt.Fprintf(f.IOStreams.Err, "Exported %d issues...\n", exported)
			}
		}

		if results.IsLast || len(results.Issues) == 0 {
			break
		}
		if opts.Limit > 0 && exported >= opts.Limit {
			break
		}
		token = results.NextPageToken
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	if f.DryRun {
		payload := map[string]interface{}{
			"output_dir": opts.OutputDir,
			"exported":   exported,
			"files":      files,
		}
		if formatter.IsJSON() {
			return formatter.OutputDryRun(payload, "passed", nil)
		}
		return formatter.OutputDryRun(nil, "", func(tw table.Writer) {
			fmt.Fprintf(f.IOStreams.Out, "DRY RUN — export preview\n\n")
			for _, file := range files {
				fmt.Fprintf(f.IOStreams.Out, "  %s\n", file)
			}
			fmt.Fprintf(f.IOStreams.Out, "\nWould export %d issues to %s\n", exported, opts.OutputDir)
		})
	}

	if formatter.IsJSON() {
		extras := map[string]interface{}{
			"exported":   exported,
			"output_dir": opts.OutputDir,
			"files":      files,
		}
		return formatter.OutputMutation(extras, nil)
	}

	// Text output.
	return formatter.OutputMutation(nil, func(tw table.Writer) {
		fmt.Fprintf(f.IOStreams.Out, "Exported %d issues to %s\n", exported, opts.OutputDir)
	})
}

// buildExportJQL constructs JQL for the export command.
func buildExportJQL(f *factory.Factory, opts *ExportOptions) (string, error) {
	if opts.JQL != "" {
		return opts.JQL, nil
	}
	project, err := resolveProject(f, opts.Project)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("project = '%s' ORDER BY key ASC", project), nil
}

// writeFileAtomic writes an issue to a markdown file using temp-then-rename.
func writeFileAtomic(path string, issue api.Issue) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return clierrors.NewGeneralError(fmt.Sprintf("create directory %s", dir)).WithErr(err)
	}

	data, err := markdown.IssueToMarkdown(issue)
	if err != nil {
		return clierrors.NewGeneralError(fmt.Sprintf("convert issue %s to markdown", issue.Key)).WithErr(err)
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return clierrors.NewGeneralError(fmt.Sprintf("write temp file %s", tmpPath)).WithErr(err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // best-effort cleanup
		return clierrors.NewGeneralError(fmt.Sprintf("rename %s to %s", tmpPath, path)).WithErr(err)
	}

	return nil
}
