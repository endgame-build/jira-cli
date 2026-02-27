package meta

import (
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/output"
)

// CommandInfo describes a single CLI command for machine consumption.
type CommandInfo struct {
	Command     string     `json:"command"`
	Description string     `json:"description"`
	Args        []ArgInfo  `json:"args"`
	Flags       []FlagInfo `json:"flags"`
}

// ArgInfo describes a positional argument.
type ArgInfo struct {
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// FlagInfo describes a command flag.
type FlagInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Default     string `json:"default"`
	Description string `json:"description"`
}

// CommandsOptions holds all resolved inputs for the meta commands command.
type CommandsOptions struct {
	Factory *factory.Factory
	Root    *cobra.Command
}

// NewCmdCommands creates the "meta commands" command.
func NewCmdCommands(f *factory.Factory) *cobra.Command {
	opts := &CommandsOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "commands",
		Short: "List all CLI commands and their flags",
		Long:  "Discover the entire CLI surface — commands, arguments, and flags — for programmatic consumption by LLM agents.",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Root = cmd.Root()
			return runMetaCommands(opts)
		},
	}

	return cmd
}

// runMetaCommands walks the Cobra command tree and outputs command metadata.
func runMetaCommands(opts *CommandsOptions) error {
	f := opts.Factory

	commands := walkCommands(opts.Root, "")

	if f.Quiet {
		return nil
	}

	// --text: table output. Otherwise JSON is the default for meta commands.
	if f.Text {
		formatter := output.NewFormatter(f.IOStreams, false, "")
		return formatter.OutputData(commands, func(tw table.Writer) {
			tw.AppendHeader(table.Row{"COMMAND", "DESCRIPTION", "FLAGS"})

			for _, cmd := range commands {
				flags := requiredFlagsSummary(cmd.Flags)
				tw.AppendRow(table.Row{cmd.Command, cmd.Description, flags})
			}
		})
	}

	// JSON mode (default): bare array. --jq works here too.
	formatter := output.NewFormatter(f.IOStreams, true, f.JQExpr)
	return formatter.RawJSON(commands)
}

// walkCommands recursively walks the Cobra command tree and collects CommandInfo.
func walkCommands(cmd *cobra.Command, parentPath string) []CommandInfo {
	var result []CommandInfo

	for _, child := range cmd.Commands() {
		if child.Hidden {
			continue
		}

		fullPath := child.Name()
		if parentPath != "" {
			fullPath = parentPath + " " + child.Name()
		}

		// If the command has subcommands, recurse into them.
		if child.HasSubCommands() {
			result = append(result, walkCommands(child, fullPath)...)
			continue
		}

		// Leaf command — collect its info.
		info := CommandInfo{
			Command:     "jira " + fullPath,
			Description: child.Short,
			Args:        extractArgs(child),
			Flags:       extractFlags(child),
		}

		result = append(result, info)
	}

	return result
}

// extractArgs parses positional arguments from the command's Use string.
// Cobra's Use field follows the format: "name <required> [optional]".
func extractArgs(cmd *cobra.Command) []ArgInfo {
	var args []ArgInfo

	use := cmd.Use
	// Strip the command name (first word).
	parts := strings.Fields(use)
	if len(parts) <= 1 {
		return args
	}

	for _, part := range parts[1:] {
		// Skip flags placeholder like "[flags]".
		if strings.EqualFold(part, "[flags]") {
			continue
		}

		name := part
		required := false

		if strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">") {
			name = strings.TrimPrefix(strings.TrimSuffix(part, ">"), "<")
			required = true
		} else if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
			name = strings.TrimPrefix(strings.TrimSuffix(part, "]"), "[")
			required = false
		}

		args = append(args, ArgInfo{
			Name:     name,
			Required: required,
		})
	}

	return args
}

// RequiredAnnotation is a custom flag annotation key used to signal that a
// flag is required for meta commands discovery, without triggering Cobra's
// built-in required-flag enforcement (which would replace custom CLIError
// messages with Cobra's generic error).
const RequiredAnnotation = "jira_required"

// MarkRequired marks a flag as required for meta commands discovery.
// Unlike Cobra's MarkFlagRequired, this does not change runtime validation.
func MarkRequired(cmd *cobra.Command, names ...string) {
	for _, name := range names {
		f := cmd.Flags().Lookup(name)
		if f == nil {
			continue
		}
		if f.Annotations == nil {
			f.Annotations = map[string][]string{}
		}
		f.Annotations[RequiredAnnotation] = []string{"true"}
	}
}

// extractFlags collects non-hidden flags from the command.
func extractFlags(cmd *cobra.Command) []FlagInfo {
	var flags []FlagInfo

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}

		required := false
		// Check our custom annotation first, then Cobra's built-in.
		if annot, ok := f.Annotations[RequiredAnnotation]; ok {
			if len(annot) > 0 && annot[0] == "true" {
				required = true
			}
		} else if annot, ok := f.Annotations[cobra.BashCompOneRequiredFlag]; ok {
			if len(annot) > 0 && annot[0] == "true" {
				required = true
			}
		}

		flags = append(flags, FlagInfo{
			Name:        f.Name,
			Type:        f.Value.Type(),
			Required:    required,
			Default:     f.DefValue,
			Description: f.Usage,
		})
	})

	return flags
}

// requiredFlagsSummary returns a comma-separated list of required flags, or "(none)".
func requiredFlagsSummary(flags []FlagInfo) string {
	var required []string
	for _, f := range flags {
		if f.Required {
			required = append(required, "--"+f.Name)
		}
	}
	if len(required) == 0 {
		return "(none)"
	}
	return strings.Join(required, ", ")
}
