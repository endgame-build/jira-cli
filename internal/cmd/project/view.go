package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// ProjectViewOptions holds all resolved inputs for the project view command.
type ProjectViewOptions struct {
	Factory *factory.Factory
	KeyOrID string
}

// NewCmdView creates the "project view" command.
func NewCmdView(f *factory.Factory) *cobra.Command {
	opts := &ProjectViewOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "view <key-or-id>",
		Short: "View a Jira project",
		Long:  "Display details of a Jira project by key (e.g. PROJ) or numeric ID.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.KeyOrID = args[0]
			return runProjectView(opts)
		},
	}

	return cmd
}

// runProjectView fetches a project and renders it.
func runProjectView(opts *ProjectViewOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	project, err := client.GetProject(ctx, opts.KeyOrID)
	if err != nil {
		return err
	}

	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)

	if f.Quiet {
		return nil
	}

	// JSON mode: bare object output (no envelope).
	if formatter.IsJSON() {
		return formatter.RawJSON(project)
	}

	// Text mode: key-value display.
	return formatter.OutputData(project, func(tw table.Writer) {
		fmt.Fprintf(f.IOStreams.Out, "Key:          %s\n", project.Key)
		fmt.Fprintf(f.IOStreams.Out, "Name:         %s\n", project.Name)

		lead := "(none)"
		if project.Lead != nil {
			lead = project.Lead.DisplayName
		}
		fmt.Fprintf(f.IOStreams.Out, "Lead:         %s\n", lead)

		if project.Description != "" {
			desc := project.Description
			lines := strings.Split(desc, "\n")
			if len(lines) > 5 {
				lines = append(lines[:5], "... (truncated)")
			}
			fmt.Fprintf(f.IOStreams.Out, "Description:  %s\n", strings.Join(lines, "\n              "))
		}

		fmt.Fprintf(f.IOStreams.Out, "Type:         %s\n", project.ProjectTypeKey)

		if len(project.IssueTypes) > 0 {
			names := make([]string, len(project.IssueTypes))
			for i, it := range project.IssueTypes {
				names[i] = it.Name
			}
			fmt.Fprintf(f.IOStreams.Out, "Issue Types:  %s\n", strings.Join(names, ", "))
		}

		if project.URL != "" {
			fmt.Fprintf(f.IOStreams.Out, "URL:          %s\n", project.URL)
		}
	})
}
