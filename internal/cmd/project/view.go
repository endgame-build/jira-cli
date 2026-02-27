package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/cmd/shared"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
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
			key, err := shared.ValidateProjectKeyOrID(args[0])
			if err != nil {
				return err
			}
			opts.KeyOrID = key
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

	// Construct browse URL client-side if the API didn't provide one.
	// Jira's /project endpoint returns an optional "url" field for external
	// project links, not the browse URL. We build it from instance + key.
	if project.URL == "" {
		project.URL = fmt.Sprintf("https://%s/browse/%s", client.Instance(), project.Key)
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
	out := f.IOStreams.Out
	fmt.Fprintf(out, "Key:          %s\n", project.Key)
	fmt.Fprintf(out, "Name:         %s\n", project.Name)

	lead := "(none)"
	if project.Lead != nil {
		lead = project.Lead.DisplayName
	}
	fmt.Fprintf(out, "Lead:         %s\n", lead)

	if project.Description != "" {
		desc := project.Description
		lines := strings.Split(desc, "\n")
		if len(lines) > 5 {
			lines = append(lines[:5], "... (truncated)")
		}
		fmt.Fprintf(out, "Description:  %s\n", strings.Join(lines, "\n              "))
	}

	fmt.Fprintf(out, "Type:         %s\n", project.ProjectTypeKey)

	if len(project.IssueTypes) > 0 {
		names := make([]string, len(project.IssueTypes))
		for i, it := range project.IssueTypes {
			names[i] = it.Name
		}
		fmt.Fprintf(out, "Issue Types:  %s\n", strings.Join(names, ", "))
	}

	fmt.Fprintf(out, "URL:          %s\n", project.URL)

	return nil
}
