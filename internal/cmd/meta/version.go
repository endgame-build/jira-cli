package meta

import (
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/output"
)

// VersionInfo describes the CLI version for machine consumption.
type VersionInfo struct {
	Version  string  `json:"version"`
	API      string  `json:"api"`
	Instance *string `json:"instance"`
}

// VersionOptions holds all resolved inputs for the meta version command.
type VersionOptions struct {
	Factory *factory.Factory
	Root    *cobra.Command
}

// NewCmdVersion creates the "meta version" command.
func NewCmdVersion(f *factory.Factory) *cobra.Command {
	opts := &VersionOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show CLI version and API compatibility",
		Long:  "Display the CLI version, API target, and configured Jira instance — useful for agents to check compatibility.",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Root = cmd.Root()
			return runMetaVersion(opts)
		},
	}

	return cmd
}

// runMetaVersion outputs version metadata.
func runMetaVersion(opts *VersionOptions) error {
	f := opts.Factory

	version := opts.Root.Version
	if version == "" {
		version = "dev"
	}

	// Resolve instance WITHOUT triggering auth.
	var instance *string
	if inst := f.ResolveInstance(); inst != "" {
		instance = &inst
	}

	info := VersionInfo{
		Version:  version,
		API:      "jira-cloud-v3",
		Instance: instance,
	}

	if f.Quiet {
		return nil
	}

	// --text: human-readable output.
	if f.Text {
		formatter := output.NewFormatter(f.IOStreams, false, "")
		return formatter.OutputData(info, func(tw table.Writer) {
			tw.AppendHeader(table.Row{"PROPERTY", "VALUE"})
			tw.AppendRow(table.Row{"Version", info.Version})
			tw.AppendRow(table.Row{"API", info.API})
			instStr := "(not configured)"
			if info.Instance != nil {
				instStr = *info.Instance
			}
			tw.AppendRow(table.Row{"Instance", instStr})
		})
	}

	// JSON mode (default): bare object. --jq works here too.
	formatter := output.NewFormatter(f.IOStreams, true, f.JQExpr)
	return formatter.RawJSON(info)
}
