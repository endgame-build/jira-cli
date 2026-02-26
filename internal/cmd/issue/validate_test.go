package issue

import (
	stderrors "errors"
	"testing"

	clierrors "github.com/endgameio/jira-cli/internal/errors"
)

func TestValidateIssueKeyOrID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		// Valid issue keys — delegates to shared.ValidateIssueKeyOrID
		{name: "standard key", input: "PROJ-123", want: "PROJ-123"},
		{name: "lowercase key", input: "proj-123", want: "PROJ-123"},
		{name: "numeric ID", input: "10001", want: "10001"},

		// Invalid formats
		{name: "empty string", input: "", wantErr: true},
		{name: "just hyphen", input: "-", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateIssueKeyOrID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got %q", tt.input, got)
				}
				var cliErr *clierrors.CLIError
				if !stderrors.As(err, &cliErr) {
					t.Fatalf("expected CLIError, got %T", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
