package schema

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/endgame-build/jira-cli/internal/api"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/markdown"
)

// FieldValuesOptions holds all resolved inputs for the schema field-values command.
type FieldValuesOptions struct {
	Factory *factory.Factory

	Project string // --project (required)
	Output  string // --output (default: .jira-field-values.json)
}

// NewCmdFieldValues creates the "schema field-values" command.
func NewCmdFieldValues(f *factory.Factory) *cobra.Command {
	opts := &FieldValuesOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "field-values",
		Short: "Build field value mappings from existing issues",
		Long:  "Search existing Jira issues and collect raw API values for custom fields that use object format (team, option, user). Writes a .jira-field-values.json sidecar file for use by 'issue import'.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Project == "" {
				return clierrors.NewValidationError("--project is required").
					WithSuggestion("Specify the project to scan: jira schema field-values --project PROJ")
			}
			return runFieldValues(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Project, "project", "p", "", "Project key to scan for field values (required)")
	cmd.Flags().StringVarP(&opts.Output, "output", "o", markdown.FieldValuesFileName, "Output path for the field values sidecar file")

	return cmd
}

// runFieldValues scans issues in a project and builds field value mappings.
func runFieldValues(opts *FieldValuesOptions) error {
	f := opts.Factory
	ctx := context.Background()

	client, err := f.APIClient()
	if err != nil {
		return err
	}

	// Fetch field metadata.
	allFields, err := client.ListFields(ctx)
	if err != nil {
		return err
	}

	fieldMap := make(map[string]api.Field, len(allFields))
	for _, field := range allFields {
		fieldMap[field.ID] = field
	}

	// Search all issues in the project.
	jql := fmt.Sprintf("project = '%s' ORDER BY key ASC", opts.Project)
	token := ""
	allRawValues := make(markdown.FieldValueMap)
	scanned := 0

	for {
		results, err := client.SearchIssues(ctx, &api.SearchOptions{
			JQL:           jql,
			MaxResults:    50,
			NextPageToken: token,
		})
		if err != nil {
			return err
		}

		for _, issue := range results.Issues {
			for fieldID, raw := range issue.Fields.CustomFields {
				field, ok := fieldMap[fieldID]
				if !ok {
					continue
				}

				key := markdown.NormalizeFieldName(field.Name)
				if key == "" || markdown.IsBuiltinKey(key) {
					continue
				}

				// Try to extract a display value and raw object.
				var v interface{}
				if err := json.Unmarshal(raw, &v); err != nil {
					continue
				}

				objMap, isObj := v.(map[string]interface{})
				if !isObj {
					continue // Only collect object-type values.
				}

				// Determine display value (same logic as extractCustomFieldValue).
				var display string
				if value, ok := objMap["value"]; ok {
					display = fmt.Sprintf("%v", value)
				} else if name, ok := objMap["name"]; ok {
					display = fmt.Sprintf("%v", name)
				} else {
					continue
				}

				if allRawValues[key] == nil {
					allRawValues[key] = make(map[string]json.RawMessage)
				}
				if _, exists := allRawValues[key][display]; !exists {
					allRawValues[key][display] = raw
				}
			}
			scanned++
		}

		if scanned%100 == 0 && scanned > 0 {
			fmt.Fprintf(f.IOStreams.Err, "Scanned %d issues...\n", scanned)
		}

		if results.IsLast || len(results.Issues) == 0 {
			break
		}
		token = results.NextPageToken
	}

	if len(allRawValues) == 0 {
		fmt.Fprintf(f.IOStreams.Err, "No object-type custom field values found in %d issues\n", scanned)
		return nil
	}

	// Merge with existing sidecar if present.
	outputPath := opts.Output
	if !filepath.IsAbs(outputPath) {
		// Keep relative path as-is.
	}
	existing, err := markdown.LoadFieldValues(outputPath)
	if err != nil {
		return err
	}
	existing.Merge(allRawValues)

	if err := markdown.SaveFieldValues(outputPath, existing); err != nil {
		return err
	}

	// Summary.
	totalValues := 0
	for _, vals := range existing {
		totalValues += len(vals)
	}
	fmt.Fprintf(f.IOStreams.Out, "Scanned %d issues, collected %d unique values across %d fields\n",
		scanned, totalValues, len(existing))
	fmt.Fprintf(f.IOStreams.Out, "Written to %s\n", outputPath)

	return nil
}
