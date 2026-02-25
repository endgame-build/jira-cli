package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/itchyny/gojq"
)

// ApplyJQ filters jsonData with a jq expression and writes raw text output to w.
// Matches gh CLI --jq behavior: strings are printed unquoted, other values as JSON.
func ApplyJQ(w io.Writer, jsonData []byte, expr string) error {
	query, err := gojq.Parse(expr)
	if err != nil {
		return fmt.Errorf("invalid jq expression: %w", err)
	}

	var input interface{}
	if err := json.Unmarshal(jsonData, &input); err != nil {
		return fmt.Errorf("jq: invalid JSON input: %w", err)
	}

	iter := query.Run(input)
	var results []string
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return fmt.Errorf("jq: %w", err)
		}
		results = append(results, formatJQValue(v))
	}

	if len(results) > 0 {
		_, err := fmt.Fprintln(w, strings.Join(results, "\n"))
		return err
	}
	return nil
}

// formatJQValue formats a jq result value: strings are unquoted, others as JSON.
func formatJQValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case nil:
		return "null"
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}
