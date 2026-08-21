package issue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/api"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/mapping"
	"github.com/endgame-build/jira-cli/internal/output"
)

// PullOptions holds resolved inputs for the issue pull command.
type PullOptions struct {
	Factory *factory.Factory
	Files   []string
	Dir     string
	Map     string
}

// pullResult records what a single document's reconciliation changed.
type pullResult struct {
	Key      string `json:"key"`
	Path     string `json:"path"`
	Status   string `json:"status,omitempty"`
	Assignee string `json:"assignee,omitempty"`
}

// NewCmdPull creates the "issue pull" command — the JIRA-first half of the sync:
// reconcile status and assignee from JIRA back into hub-style documents, leaving
// all other frontmatter and the body untouched.
func NewCmdPull(f *factory.Factory) *cobra.Command {
	opts := &PullOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "pull [files...]",
		Short: "Reconcile status/assignee from JIRA into hub documents (JIRA-first)",
		Long: "For each document with a real jira_key, fetch its JIRA status and assignee and write " +
			"them back into the document's frontmatter per the --map config's pull rules. Only the " +
			"pulled fields are touched — content stays hub-owned. The reverse of 'issue import --map'.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Map == "" {
				return clierrors.NewValidationError("--map is required for pull").
					WithSuggestion("pull reconciles per a jira-sync.yaml field-map: --map path/to/jira-sync.yaml")
			}
			if len(args) > 0 && opts.Dir != "" {
				return clierrors.NewValidationError("specify files or --dir, not both")
			}
			if len(args) == 0 && opts.Dir == "" {
				return clierrors.NewValidationError("specify files or --dir")
			}
			opts.Files = args
			return runPull(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Dir, "dir", "d", "", "Reconcile all .md files in a directory")
	cmd.Flags().StringVar(&opts.Map, "map", "", "Path to a jira-sync.yaml field-map (required)")
	return cmd
}

func runPull(opts *PullOptions) error {
	f := opts.Factory
	ctx := context.Background()

	cfg, err := mapping.LoadConfig(opts.Map)
	if err != nil {
		return err
	}

	paths, err := collectMarkdownPaths(opts.Dir, opts.Files)
	if err != nil {
		return err
	}

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	wantStatus := cfg.WantsPull("status")
	wantAssignee := cfg.WantsPull("assignee")

	var results []pullResult
	for _, path := range paths {
		key, err := mapping.DocJiraKey(path)
		if err != nil {
			return err
		}
		if key == "" { // unmapped doc (never pushed) — nothing to reconcile
			continue
		}

		issue, err := client.GetIssue(ctx, key, &api.GetIssueOptions{Fields: []string{"status", "assignee"}})
		if err != nil {
			return err
		}

		var pairs [][2]string
		res := pullResult{Key: key, Path: path}

		if wantStatus && issue.Fields.Status != nil {
			cat := ""
			if issue.Fields.Status.StatusCategory != nil {
				cat = issue.Fields.Status.StatusCategory.Key
			}
			if hub := cfg.MapStatus(issue.Fields.Status.Name, cat); hub != "" {
				pairs = append(pairs, [2]string{"status", hub})
				res.Status = hub
			}
		}

		if wantAssignee {
			assignee := assigneeValue(issue.Fields.Assignee, cfg.Pull.AssigneeAs)
			pairs = append(pairs, [2]string{"assignee", assignee})
			res.Assignee = assignee
		}

		if len(pairs) == 0 {
			continue
		}
		if f.DryRun {
			results = append(results, res)
			continue
		}
		if err := mapping.SetFrontmatterFields(path, pairs); err != nil {
			return err
		}
		results = append(results, res)
	}

	return renderPull(f, results)
}

// assigneeValue extracts the hub assignee value from a JIRA user per the config's
// assignee_as preference. Returns "" for an unassigned issue (written as null).
func assigneeValue(u *api.User, as string) string {
	if u == nil {
		return ""
	}
	if as == "account_id" {
		return u.AccountID
	}
	if u.EmailAddress != nil && *u.EmailAddress != "" {
		return *u.EmailAddress
	}
	if u.DisplayName != "" { // email masked by privacy — fall back to a stable identifier
		return u.DisplayName
	}
	return u.AccountID
}

// collectMarkdownPaths returns the sorted .md files from a directory or the explicit list.
func collectMarkdownPaths(dir string, files []string) ([]string, error) {
	if dir == "" {
		return files, nil
	}
	var paths []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".md") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, clierrors.NewValidationError("failed to walk directory: " + dir).WithErr(err)
	}
	sort.Strings(paths)
	return paths, nil
}

func renderPull(f *factory.Factory, results []pullResult) error {
	formatter := output.NewFormatter(f.IOStreams, f.OutputJSON, f.JQExpr)
	if f.Quiet {
		return nil
	}
	if formatter.IsJSON() {
		return formatter.OutputMutation(map[string]interface{}{
			"reconciled": len(results),
			"results":    results,
		}, nil)
	}
	verb := "Reconciled"
	if f.DryRun {
		verb = "Would reconcile"
	}
	return formatter.OutputMutation(nil, func(tw table.Writer) {
		for _, r := range results {
			fmt.Fprintf(f.IOStreams.Out, "%s %s: status=%s assignee=%s\n",
				verb, r.Key, dash(r.Status), dash(r.Assignee))
		}
		fmt.Fprintf(f.IOStreams.Out, "%s %d document(s)\n", verb, len(results))
	})
}

func dash(s string) string {
	if s == "" {
		return "null"
	}
	return s
}
