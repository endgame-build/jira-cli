package schema

import (
	"context"
	"fmt"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// TypesOptions holds all resolved inputs for the schema types command.
type TypesOptions struct {
	Factory *factory.Factory

	Project string // --project (optional, scopes to project)
}

// NewCmdTypes creates the "schema types" command.
func NewCmdTypes(f *factory.Factory) *cobra.Command {
	opts := &TypesOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "types",
		Short: "List issue types",
		Long:  "List available issue types, optionally scoped to a project.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSchemaTypes(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Project, "project", "", "Scope to a specific project key or ID")

	return cmd
}

// runSchemaTypes executes the schema types workflow.
func runSchemaTypes(opts *TypesOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	var types []api.IssueType
	if opts.Project != "" {
		// Resolve project key to numeric ID via GetProject.
		project, err := client.GetProject(ctx, opts.Project)
		if err != nil {
			return err
		}
		types, err = client.ListIssueTypesForProject(ctx, project.ID)
		if err != nil {
			return err
		}
	} else {
		types, err = client.ListIssueTypes(ctx)
		if err != nil {
			return err
		}
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	// JSON mode: unpaginated list envelope (pagination: null).
	if formatter.IsJSON() {
		return formatter.OutputList(types, nil, nil)
	}

	// Text mode: table output.
	if len(types) == 0 {
		fmt.Fprintln(f.IOStreams.Out, "No issue types found")
		return nil
	}

	return formatter.OutputList(types, nil, func(tw table.Writer) {
		tw.AppendHeader(table.Row{"NAME", "DESCRIPTION", "SUBTASK", "ICON URL"})

		for _, it := range types {
			subtask := "no"
			if it.Subtask {
				subtask = "yes"
			}
			desc := truncateDescription(it.Description, 60)
			tw.AppendRow(table.Row{it.Name, desc, subtask, it.IconURL})
		}
	})
}

// truncateDescription truncates a string to maxLen characters with an ellipsis.
func truncateDescription(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
