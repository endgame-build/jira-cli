package output

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/endgameio/jira-cli/internal/iostreams"
)

// Formatter handles output routing between JSON and text (table) modes.
// Commands call its methods instead of writing directly to stdout.
type Formatter struct {
	ios    *iostreams.IOStreams
	asJSON bool
	jqExpr string
}

// NewFormatter creates a Formatter. asJSON enables JSON output, jqExpr applies
// a jq filter (implies JSON mode).
func NewFormatter(ios *iostreams.IOStreams, asJSON bool, jqExpr string) *Formatter {
	return &Formatter{
		ios:    ios,
		asJSON: asJSON || jqExpr != "",
		jqExpr: jqExpr,
	}
}

// IsJSON reports whether the formatter is in JSON mode.
func (f *Formatter) IsJSON() bool {
	return f.asJSON
}

// OutputData renders a single data object: bare JSON or text table.
func (f *Formatter) OutputData(data interface{}, tableFn TableFunc) error {
	if f.asJSON {
		return f.outputJSONOrJQ(func(w io.Writer) error {
			return writeDataJSON(w, data)
		})
	}
	renderTable(f.ios.Out, tableFn)
	return nil
}

// OutputList renders a list with pagination: JSON envelope or text table.
func (f *Formatter) OutputList(items interface{}, pagination *PaginationMeta, tableFn TableFunc) error {
	if f.asJSON {
		return f.outputJSONOrJQ(func(w io.Writer) error {
			return writeListJSON(w, items, pagination)
		})
	}
	renderTable(f.ios.Out, tableFn)
	return nil
}

// OutputMutation renders a mutation result: {"ok":true, ...} or text.
func (f *Formatter) OutputMutation(extras map[string]interface{}, tableFn TableFunc) error {
	if f.asJSON {
		return f.outputJSONOrJQ(func(w io.Writer) error {
			return writeMutationJSON(w, extras)
		})
	}
	renderTable(f.ios.Out, tableFn)
	return nil
}

// OutputDryRun renders a dry-run preview. JSON includes dry_run:true plus payload
// and validation. Text renders via the supplied table function.
func (f *Formatter) OutputDryRun(payload interface{}, validation string, tableFn TableFunc) error {
	return f.OutputDryRunWithContext(nil, payload, validation, tableFn)
}

// OutputDryRunWithContext renders a dry-run preview with optional extra context
// fields merged at the JSON top level (e.g., key, comment_id). Text output is
// unaffected by extras.
func (f *Formatter) OutputDryRunWithContext(extras map[string]interface{}, payload interface{}, validation string, tableFn TableFunc) error {
	if f.asJSON {
		result := make(map[string]interface{}, 3+len(extras))
		for k, v := range extras {
			result[k] = v
		}
		// Reserved keys always take precedence over extras.
		result["dry_run"] = true
		result["payload"] = payload
		result["validation"] = validation
		return f.outputJSONOrJQ(func(w io.Writer) error {
			return writeJSON(w, result)
		})
	}
	renderTable(f.ios.Out, tableFn)
	return nil
}

// outputJSONOrJQ captures JSON into a buffer, then either applies jq or writes directly.
func (f *Formatter) outputJSONOrJQ(writeFn func(w io.Writer) error) error {
	if f.jqExpr == "" {
		return writeFn(f.ios.Out)
	}

	var buf bytes.Buffer
	if err := writeFn(&buf); err != nil {
		return err
	}

	// json.Encoder adds a trailing newline; remove it for clean jq input.
	raw := bytes.TrimRight(buf.Bytes(), "\n")
	return ApplyJQ(f.ios.Out, raw, f.jqExpr)
}

// RawJSON serializes data to JSON and writes it. Useful for pass-through output.
func (f *Formatter) RawJSON(data interface{}) error {
	return f.outputJSONOrJQ(func(w io.Writer) error {
		return writeJSON(w, data)
	})
}

// MarshalForJQ serializes data to JSON bytes, suitable for ApplyJQ input.
func MarshalForJQ(data interface{}) ([]byte, error) {
	return json.Marshal(data)
}
