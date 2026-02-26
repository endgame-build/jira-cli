package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	clierrors "github.com/endgameio/jira-cli/internal/errors"
)

// OutputError renders err to w (typically stderr).
// When asJSON is true, outputs the CLIError JSON envelope.
// When asJSON is false, outputs human-readable text with suggestion.
func OutputError(w io.Writer, err error, asJSON bool) {
	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		cliErr = clierrors.NewGeneralError(err.Error())
	}

	if asJSON {
		b, marshalErr := json.MarshalIndent(cliErr, "", "  ")
		if marshalErr != nil {
			// Fallback: raw error text if marshaling somehow fails.
			fmt.Fprintf(w, "Error: %s\n", err)
			return
		}
		fmt.Fprintln(w, string(b))
		return
	}

	// Text mode: "Error: message\nSuggestion: ..."
	fmt.Fprintf(w, "Error: %s\n", cliErr.Message)
	if cliErr.Suggestion != "" {
		fmt.Fprintf(w, "Suggestion: %s\n", cliErr.Suggestion)
	}
}
