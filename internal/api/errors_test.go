package api

import (
	"errors"
	"testing"

	cliErrors "github.com/endgameio/jira-cli/internal/errors"
)

func TestParseErrorCollection_ValidJSON(t *testing.T) {
	body := []byte(`{"errorMessages":["Issue does not exist"],"errors":{}}`)
	ec := parseErrorCollection(body)
	if ec == nil {
		t.Fatal("expected non-nil ErrorCollection")
	}
	if len(ec.ErrorMessages) != 1 || ec.ErrorMessages[0] != "Issue does not exist" {
		t.Errorf("ErrorMessages = %v, want [\"Issue does not exist\"]", ec.ErrorMessages)
	}
}

func TestParseErrorCollection_FieldErrors(t *testing.T) {
	body := []byte(`{"errorMessages":[],"errors":{"summary":"Field is required","project":"Project is required"}}`)
	ec := parseErrorCollection(body)
	if ec == nil {
		t.Fatal("expected non-nil ErrorCollection")
	}
	if ec.Errors["summary"] != "Field is required" {
		t.Errorf("errors.summary = %q, want %q", ec.Errors["summary"], "Field is required")
	}
	if ec.Errors["project"] != "Project is required" {
		t.Errorf("errors.project = %q, want %q", ec.Errors["project"], "Project is required")
	}
}

func TestParseErrorCollection_Both(t *testing.T) {
	body := []byte(`{"errorMessages":["Something wrong"],"errors":{"summary":"Required"}}`)
	ec := parseErrorCollection(body)
	if ec == nil {
		t.Fatal("expected non-nil ErrorCollection")
	}
	summary := ec.Summary()
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
	// Both the message and field error should appear.
	if !containsSubstring(summary, "Something wrong") {
		t.Errorf("summary missing 'Something wrong': %q", summary)
	}
	if !containsSubstring(summary, "summary: Required") {
		t.Errorf("summary missing 'summary: Required': %q", summary)
	}
}

func TestParseErrorCollection_EmptyBody(t *testing.T) {
	ec := parseErrorCollection(nil)
	if ec != nil {
		t.Error("expected nil for empty body")
	}
	ec = parseErrorCollection([]byte{})
	if ec != nil {
		t.Error("expected nil for zero-length body")
	}
}

func TestParseErrorCollection_InvalidJSON(t *testing.T) {
	ec := parseErrorCollection([]byte(`not json`))
	if ec != nil {
		t.Error("expected nil for invalid JSON")
	}
}

func TestParseErrorCollection_EmptyCollections(t *testing.T) {
	ec := parseErrorCollection([]byte(`{"errorMessages":[],"errors":{}}`))
	if ec != nil {
		t.Error("expected nil when both collections are empty")
	}
}

func TestParseErrorCollection_NoFields(t *testing.T) {
	// JSON with unrelated fields should return nil.
	ec := parseErrorCollection([]byte(`{"foo":"bar"}`))
	if ec != nil {
		t.Error("expected nil for JSON without error fields")
	}
}

func TestSummary_OnlyMessages(t *testing.T) {
	ec := &ErrorCollection{
		ErrorMessages: []string{"First error", "Second error"},
	}
	want := "First error; Second error"
	if got := ec.Summary(); got != want {
		t.Errorf("Summary() = %q, want %q", got, want)
	}
}

func TestSummary_OnlyFieldErrors(t *testing.T) {
	ec := &ErrorCollection{
		Errors: map[string]string{"summary": "Required"},
	}
	got := ec.Summary()
	if got != "summary: Required" {
		t.Errorf("Summary() = %q, want %q", got, "summary: Required")
	}
}

func TestSummary_Empty(t *testing.T) {
	ec := &ErrorCollection{}
	if got := ec.Summary(); got != "" {
		t.Errorf("Summary() = %q, want empty", got)
	}
}

func TestMapHTTPError_StatusCodes(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantCode cliErrors.ErrorCode
	}{
		{
			name:     "400 validation",
			status:   400,
			body:     `{"errorMessages":["Field X is invalid"],"errors":{}}`,
			wantCode: cliErrors.VALIDATION_ERROR,
		},
		{
			name:     "401 auth",
			status:   401,
			body:     `{"errorMessages":["Unauthorized"],"errors":{}}`,
			wantCode: cliErrors.AUTH_ERROR,
		},
		{
			name:     "403 permission",
			status:   403,
			body:     `{"errorMessages":["Forbidden"],"errors":{}}`,
			wantCode: cliErrors.PERMISSION_DENIED,
		},
		{
			name:     "404 not found",
			status:   404,
			body:     `{"errorMessages":["Issue does not exist"],"errors":{}}`,
			wantCode: cliErrors.NOT_FOUND,
		},
		{
			name:     "409 conflict",
			status:   409,
			body:     `{"errorMessages":["Conflict"],"errors":{}}`,
			wantCode: cliErrors.CONFLICT_ERROR,
		},
		{
			name:     "429 rate limited",
			status:   429,
			body:     `{"errorMessages":["Rate limited"],"errors":{}}`,
			wantCode: cliErrors.RATE_LIMITED,
		},
		{
			name:     "500 server error",
			status:   500,
			body:     `{"errorMessages":["Internal Server Error"],"errors":{}}`,
			wantCode: cliErrors.GENERAL_ERROR,
		},
		{
			name:     "502 bad gateway",
			status:   502,
			body:     ``,
			wantCode: cliErrors.GENERAL_ERROR,
		},
		{
			name:     "400 with field errors",
			status:   400,
			body:     `{"errorMessages":[],"errors":{"summary":"Summary is required"}}`,
			wantCode: cliErrors.VALIDATION_ERROR,
		},
		{
			name:     "404 empty body",
			status:   404,
			body:     ``,
			wantCode: cliErrors.NOT_FOUND,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := newMockResponse(tt.status, tt.body)
			err := mapHTTPError(resp, "test.atlassian.net")

			var cliErr *cliErrors.CLIError
			if !errors.As(err, &cliErr) {
				t.Fatalf("expected CLIError, got %T: %v", err, err)
			}
			if cliErr.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", cliErr.Code, tt.wantCode)
			}
		})
	}
}

func TestMapHTTPError_MessageFromErrorCollection(t *testing.T) {
	body := `{"errorMessages":["Issue PROJ-999 does not exist"],"errors":{}}`
	resp := newMockResponse(404, body)
	err := mapHTTPError(resp, "test.atlassian.net")

	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if !containsSubstring(cliErr.Message, "PROJ-999") {
		t.Errorf("message = %q, want it to contain 'PROJ-999'", cliErr.Message)
	}
}

func TestMapHTTPError_FallbackMessage(t *testing.T) {
	// Non-parseable body → falls back to HTTP status text.
	resp := newMockResponse(403, "not json")
	err := mapHTTPError(resp, "test.atlassian.net")

	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if !containsSubstring(cliErr.Message, "403") {
		t.Errorf("message = %q, want it to contain '403'", cliErr.Message)
	}
}

func TestMapHTTPError_5xxIncludesInstanceContext(t *testing.T) {
	resp := newMockResponse(500, `{"errorMessages":["error"],"errors":{}}`)
	err := mapHTTPError(resp, "mysite.atlassian.net")

	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Context == nil {
		t.Fatal("expected context on 5xx error")
	}
	if cliErr.Context["instance"] != "mysite.atlassian.net" {
		t.Errorf("context.instance = %v, want %q", cliErr.Context["instance"], "mysite.atlassian.net")
	}
}

func TestMapHTTPError_401HasSuggestion(t *testing.T) {
	resp := newMockResponse(401, `{"errorMessages":["Unauthorized"],"errors":{}}`)
	err := mapHTTPError(resp, "test.atlassian.net")

	var cliErr *cliErrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T", err)
	}
	if cliErr.Suggestion == "" {
		t.Error("expected suggestion on 401 error")
	}
	if !containsSubstring(cliErr.Suggestion, "login") {
		t.Errorf("suggestion = %q, want it to mention 'login'", cliErr.Suggestion)
	}
}

// containsSubstring is a test helper.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsIdx(s, substr))
}

func containsIdx(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
