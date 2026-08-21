package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/cmd/shared"
	"github.com/endgame-build/jira-cli/internal/factory"
)

// PrimeOptions holds all resolved inputs for the agent prime command.
type PrimeOptions struct {
	Factory *factory.Factory
	Project string
	Full    bool
}

// NewCmdPrime creates the "agent prime" command.
func NewCmdPrime(f *factory.Factory) *cobra.Command {
	opts := &PrimeOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "prime",
		Short: "Output workflow context for agent injection",
		Long:  "Output a self-contained markdown block with everything an agent needs to execute the agentic SDLC loop. Designed for Claude Code hooks.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrime(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Project, "project", "p", "", "Jira project key (falls back to default.project config)")
	cmd.Flags().BoolVar(&opts.Full, "full", false, "Include extended command reference")

	return cmd
}

// runPrime outputs workflow context markdown.
func runPrime(opts *PrimeOptions) error {
	f := opts.Factory
	ctx := context.Background()

	project, err := shared.ResolveProject(f, opts.Project)
	if err != nil {
		return err
	}

	// Fetch project-specific metadata.
	var statusLines []string
	var typeLines []string

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	// Get project statuses.
	issueTypeStatuses, err := client.GetProjectStatuses(ctx, project)
	if err != nil {
		fmt.Fprintf(f.IOStreams.Err, "Warning: could not fetch project statuses: %v\n", err)
		statusLines = []string{"(unable to fetch — use `jira issue transitions <key>` to check)"}
	} else {
		seen := map[string]bool{}
		for _, cat := range issueTypeStatuses {
			for _, s := range cat.Statuses {
				if !seen[s.Name] {
					seen[s.Name] = true
					category := ""
					if s.StatusCategory != nil {
						category = s.StatusCategory.Key
					}
					statusLines = append(statusLines, fmt.Sprintf("%s (%s)", s.Name, category))
				}
			}
		}
	}

	// Get active sprint (non-fatal).
	var activeSprint *sprintInfo
	if s, err := client.GetActiveSprint(ctx, project); err != nil {
		fmt.Fprintf(f.IOStreams.Err, "Warning: could not fetch sprint info: %v\n", err)
	} else {
		activeSprint = toSprintInfo(s)
	}

	// Get issue types.
	meta, err := client.GetCreateMeta(ctx, project)
	if err != nil {
		fmt.Fprintf(f.IOStreams.Err, "Warning: could not fetch issue types: %v\n", err)
		typeLines = []string{"(unable to fetch — check project settings)"}
	} else {
		for _, it := range meta.IssueTypes {
			suffix := ""
			if it.Subtask {
				suffix = " (subtask)"
			}
			typeLines = append(typeLines, it.Name+suffix)
		}
	}

	// Build output.
	var b strings.Builder

	b.WriteString("# Jira Agent Workflow Context\n\n")

	b.WriteString("## Rules\n")
	b.WriteString("- Use `jira agent` for ALL task tracking\n")
	b.WriteString("- Do NOT use TodoWrite, TaskCreate, or markdown TODO lists\n")
	b.WriteString("- Claim work before starting implementation\n")
	b.WriteString("- File discovered work with `jira agent discover`\n\n")

	b.WriteString("## Core Commands\n")
	b.WriteString("- `jira agent ready --project " + project + "` — Find unblocked work\n")
	b.WriteString("- `jira agent claim <key>` — Assign + move to In Progress\n")
	b.WriteString("- `jira agent close <key> --reason=\"...\"` — Complete work\n")
	b.WriteString("- `jira agent discover <parent-key> --title=\"...\"` — File discovered work\n")
	b.WriteString("- `jira agent status --project " + project + "` — Current work summary\n")
	b.WriteString("- `jira agent blocked --project " + project + "` — Blocked issues with details\n\n")

	if activeSprint != nil {
		b.WriteString("\n## Sprint: " + activeSprint.Name + "\n")
		if activeSprint.Goal != "" {
			b.WriteString("- **Goal:** " + activeSprint.Goal + "\n")
		}
		if activeSprint.EndDate != "" {
			b.WriteString(fmt.Sprintf("- **Ends:** %s (%d days remaining)\n", activeSprint.EndDate, activeSprint.RemainingDays))
		}
		b.WriteString("- **Tip:** Use `--sprint active` with `agent ready` to focus on sprint work\n")
	}

	b.WriteString("\n## Session Protocol\n")
	if activeSprint != nil {
		b.WriteString("1. `jira agent ready --project " + project + " --sprint active --json` — find sprint work\n")
	} else {
		b.WriteString("1. `jira agent ready --project " + project + " --json` — find work\n")
	}
	b.WriteString("2. `jira agent claim <key>` — claim it\n")
	b.WriteString("3. [implement]\n")
	b.WriteString("4. `jira agent close <key> --reason=\"...\"` — close\n")
	b.WriteString("5. `git add && git commit && git push` — push code\n\n")

	b.WriteString("## Project: " + project + "\n")
	b.WriteString("- **Statuses:** " + strings.Join(statusLines, ", ") + "\n")
	b.WriteString("- **Types:** " + strings.Join(typeLines, ", ") + "\n")
	b.WriteString("- **Priority levels:** Highest, High, Medium, Low, Lowest\n")

	if opts.Full {
		b.WriteString("\n## Extended Reference\n\n")
		b.WriteString("### Ready Queue Flags\n")
		b.WriteString("- `--assignee @me` — Only my issues\n")
		b.WriteString("- `--unassigned` — Only unassigned issues\n")
		b.WriteString("- `--type Bug` — Filter by type\n")
		b.WriteString("- `--label backend` — Filter by label\n")
		b.WriteString("- `--priority High` — Filter by priority\n")
		b.WriteString("- `--component api` — Filter by component\n")
		b.WriteString("- `--limit 20` — Max results (default 10)\n\n")

		b.WriteString("### Discover Flags\n")
		b.WriteString("- `--title \"...\"` — Summary (required)\n")
		b.WriteString("- `--description \"...\"` — Description (Markdown)\n")
		b.WriteString("- `--type Bug` — Issue type\n")
		b.WriteString("- `--priority High` — Override parent priority\n")
		b.WriteString("- `--as-subtask=false` — Create as linked issue instead\n\n")

		b.WriteString("### Close Flags\n")
		b.WriteString("- `--reason \"...\"` — Close reason (added as comment)\n")
		b.WriteString("- `--suggest-next` — Show newly unblocked work\n")
		b.WriteString("- `--claim-next` — Auto-claim top unblocked issue\n")
	}

	fmt.Fprint(f.IOStreams.Out, b.String())
	return nil
}
