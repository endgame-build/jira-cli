package shared

import (
	stderrors "errors"
	"testing"

	clierrors "github.com/endgameio/jira-cli/internal/errors"
)

func TestValidateCommentID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "valid numeric", input: "10042", want: "10042"},
		{name: "single digit", input: "1", want: "1"},
		{name: "large ID", input: "9999999", want: "9999999"},
		{name: "empty string", input: "", wantErr: true},
		{name: "non-numeric", input: "abc", wantErr: true},
		{name: "mixed", input: "123abc", wantErr: true},
		{name: "negative", input: "-1", wantErr: true},
		{name: "decimal", input: "1.5", wantErr: true},
		{name: "spaces", input: "10 42", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateCommentID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got %q", tt.input, got)
				}
				var cliErr *clierrors.CLIError
				if !stderrors.As(err, &cliErr) {
					t.Fatalf("expected CLIError, got %T", err)
				}
				if cliErr.Code != clierrors.VALIDATION_ERROR {
					t.Errorf("expected VALIDATION_ERROR, got %s", cliErr.Code)
				}
				if cliErr.ExitCode != 3 {
					t.Errorf("expected exit code 3, got %d", cliErr.ExitCode)
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

func TestValidateProjectKeyOrID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		// Valid project keys
		{name: "standard key", input: "PROJ", want: "PROJ"},
		{name: "two-letter key", input: "AB", want: "AB"},
		{name: "long key", input: "MYPROJECT", want: "MYPROJECT"},

		// Lowercase normalization
		{name: "lowercase key", input: "proj", want: "PROJ"},
		{name: "mixed case key", input: "Proj", want: "PROJ"},

		// Numeric IDs
		{name: "numeric ID", input: "10001", want: "10001"},
		{name: "single digit", input: "1", want: "1"},

		// Invalid formats
		{name: "empty string", input: "", wantErr: true},
		{name: "single letter", input: "A", wantErr: true},
		{name: "issue key format", input: "PROJ-123", wantErr: true},
		{name: "contains digits", input: "PROJ1", wantErr: true},
		{name: "contains underscore", input: "MY_PROJ", wantErr: true},
		{name: "spaces", input: "MY PROJ", wantErr: true},
		{name: "special chars", input: "PROJ@", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateProjectKeyOrID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got %q", tt.input, got)
				}
				var cliErr *clierrors.CLIError
				if !stderrors.As(err, &cliErr) {
					t.Fatalf("expected CLIError, got %T", err)
				}
				if cliErr.Code != clierrors.VALIDATION_ERROR {
					t.Errorf("expected VALIDATION_ERROR, got %s", cliErr.Code)
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

func TestValidateIssueKeyOrID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		// Valid issue keys
		{name: "standard key", input: "PROJ-123", want: "PROJ-123"},
		{name: "single letter project", input: "A-1", want: "A-1"},
		{name: "long project key", input: "MYPROJECT-99999", want: "MYPROJECT-99999"},

		// Lowercase normalization
		{name: "lowercase key", input: "proj-123", want: "PROJ-123"},
		{name: "mixed case key", input: "Proj-456", want: "PROJ-456"},
		{name: "mixed case long", input: "myProject-7", want: "MYPROJECT-7"},

		// Numeric IDs
		{name: "numeric ID", input: "10001", want: "10001"},
		{name: "single digit", input: "1", want: "1"},
		{name: "large numeric ID", input: "9999999", want: "9999999"},

		// Invalid formats
		{name: "empty string", input: "", wantErr: true},
		{name: "just hyphen", input: "-", wantErr: true},
		{name: "no digits after hyphen", input: "PROJ-", wantErr: true},
		{name: "no letters before hyphen", input: "-123", wantErr: true},
		{name: "spaces", input: "PROJ 123", wantErr: true},
		{name: "special chars", input: "PROJ@123", wantErr: true},
		{name: "double hyphen", input: "PROJ--123", wantErr: true},
		{name: "trailing text", input: "PROJ-123abc", wantErr: true},
		{name: "leading digits in key", input: "123PROJ-1", wantErr: true},
		{name: "underscore in project", input: "MY_PROJ-1", wantErr: true},
		{name: "url path", input: "/browse/PROJ-1", wantErr: true},
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
				if cliErr.Code != clierrors.VALIDATION_ERROR {
					t.Errorf("expected VALIDATION_ERROR, got %s", cliErr.Code)
				}
				if cliErr.ExitCode != 3 {
					t.Errorf("expected exit code 3, got %d", cliErr.ExitCode)
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
