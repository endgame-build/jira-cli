package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestCLIErrorImplementsError(t *testing.T) {
	e := NewAuthError("bad token")
	var _ error = e // compile-time check
	if e.Error() != "AUTH_ERROR: bad token" {
		t.Fatalf("unexpected Error(): %s", e.Error())
	}
}

func TestCLIErrorWrapping(t *testing.T) {
	inner := fmt.Errorf("connection refused")
	e := NewNetworkError("cannot reach instance", "myco.atlassian.net").WithErr(inner)

	if !errors.Is(e, inner) {
		t.Fatal("errors.Is should match the wrapped error")
	}

	var cliErr *CLIError
	if !errors.As(e, &cliErr) {
		t.Fatal("errors.As should unwrap to *CLIError")
	}
	if cliErr.Code != NETWORK_ERROR {
		t.Fatalf("expected NETWORK_ERROR, got %s", cliErr.Code)
	}

	if e.Error() != "NETWORK_ERROR: cannot reach instance: connection refused" {
		t.Fatalf("unexpected Error() with wrapped: %s", e.Error())
	}
}

func TestConstructorsCodeAndExit(t *testing.T) {
	tests := []struct {
		name     string
		err      *CLIError
		code     ErrorCode
		exitCode int
	}{
		{"GeneralError", NewGeneralError("oops"), GENERAL_ERROR, 1},
		{"AuthError", NewAuthError("bad creds"), AUTH_ERROR, 2},
		{"ValidationError", NewValidationError("bad input"), VALIDATION_ERROR, 3},
		{"NotFoundError", NewNotFoundError("not found", "PROJ-123"), NOT_FOUND, 4},
		{"PermissionError", NewPermissionError("forbidden"), PERMISSION_DENIED, 5},
		{"RateLimitError", NewRateLimitError("slow down"), RATE_LIMITED, 6},
		{"TransitionError", NewTransitionError("no match", []map[string]interface{}{
			{"id": "1", "name": "Start", "toStatus": "In Progress"},
		}), INVALID_TRANSITION, 3},
		{"MissingFieldError", NewMissingFieldError("summary", "Bug"), MISSING_FIELD, 3},
		{"UnknownTypeError", NewUnknownTypeError("unknown type"), UNKNOWN_TYPE, 3},
		{"AmbiguousUserError", NewAmbiguousUserError("2 matches", []map[string]interface{}{
			{"accountId": "abc", "displayName": "Alice", "email": "a@x.com"},
			{"accountId": "def", "displayName": "Alicia", "email": "al@x.com"},
		}), AMBIGUOUS_USER, 3},
		{"NetworkError", NewNetworkError("timeout", "myco.atlassian.net"), NETWORK_ERROR, 7},
		{"ConflictError", NewConflictError("already exists"), CONFLICT_ERROR, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.code {
				t.Errorf("expected code %s, got %s", tt.code, tt.err.Code)
			}
			if tt.err.ExitCode != tt.exitCode {
				t.Errorf("expected exit %d, got %d", tt.exitCode, tt.err.ExitCode)
			}
		})
	}
}

func TestContextShapes(t *testing.T) {
	t.Run("NOT_FOUND has key", func(t *testing.T) {
		e := NewNotFoundError("issue not found", "PROJ-123")
		if e.Context["key"] != "PROJ-123" {
			t.Fatalf("expected key=PROJ-123, got %v", e.Context["key"])
		}
	})

	t.Run("NOT_FOUND without key", func(t *testing.T) {
		e := NewNotFoundError("resource not found", "")
		if e.Context != nil {
			t.Fatalf("expected nil context, got %v", e.Context)
		}
	})

	t.Run("AMBIGUOUS_USER has matches", func(t *testing.T) {
		matches := []map[string]interface{}{
			{"accountId": "a1", "displayName": "Alice", "email": "a@x.com"},
		}
		e := NewAmbiguousUserError("ambiguous", matches)
		m, ok := e.Context["matches"].([]map[string]interface{})
		if !ok || len(m) != 1 {
			t.Fatalf("expected 1 match, got %v", e.Context["matches"])
		}
		if m[0]["accountId"] != "a1" {
			t.Fatalf("expected accountId=a1, got %v", m[0]["accountId"])
		}
	})

	t.Run("INVALID_TRANSITION has available", func(t *testing.T) {
		avail := []map[string]interface{}{
			{"id": "1", "name": "Start", "toStatus": "In Progress"},
			{"id": "2", "name": "Done", "toStatus": "Done"},
		}
		e := NewTransitionError("no match", avail)
		a, ok := e.Context["available"].([]map[string]interface{})
		if !ok || len(a) != 2 {
			t.Fatalf("expected 2 transitions, got %v", e.Context["available"])
		}
	})

	t.Run("NETWORK_ERROR has instance", func(t *testing.T) {
		e := NewNetworkError("dns failure", "bad.host.net")
		if e.Context["instance"] != "bad.host.net" {
			t.Fatalf("expected instance=bad.host.net, got %v", e.Context["instance"])
		}
	})

	t.Run("MISSING_FIELD has field and type", func(t *testing.T) {
		e := NewMissingFieldError("summary", "Bug")
		if e.Context["field"] != "summary" {
			t.Fatalf("expected field=summary, got %v", e.Context["field"])
		}
		if e.Context["type"] != "Bug" {
			t.Fatalf("expected type=Bug, got %v", e.Context["type"])
		}
	})
}

func TestJSONMarshal(t *testing.T) {
	tests := []struct {
		name string
		err  *CLIError
	}{
		{
			"auth error",
			NewAuthError("invalid credentials").WithSuggestion("Run: jira auth login"),
		},
		{
			"not found with context",
			NewNotFoundError("issue not found", "PROJ-999"),
		},
		{
			"ambiguous user with matches",
			NewAmbiguousUserError("2 matches", []map[string]interface{}{
				{"accountId": "a1", "displayName": "Alice", "email": "a@x.com"},
				{"accountId": "a2", "displayName": "Alicia", "email": "al@x.com"},
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.err)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			// Verify top-level shape: {"error": {...}}
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(data, &envelope); err != nil {
				t.Fatalf("unmarshal envelope: %v", err)
			}
			if _, ok := envelope["error"]; !ok {
				t.Fatal("missing 'error' key in JSON")
			}

			// Verify inner fields
			var inner struct {
				Code       string                 `json:"code"`
				Message    string                 `json:"message"`
				Context    map[string]interface{} `json:"context"`
				Suggestion string                 `json:"suggestion"`
			}
			if err := json.Unmarshal(envelope["error"], &inner); err != nil {
				t.Fatalf("unmarshal inner: %v", err)
			}
			if inner.Code != string(tt.err.Code) {
				t.Errorf("JSON code: expected %s, got %s", tt.err.Code, inner.Code)
			}
			if inner.Message != tt.err.Message {
				t.Errorf("JSON message: expected %q, got %q", tt.err.Message, inner.Message)
			}
		})
	}
}

func TestWithMethods(t *testing.T) {
	e := NewValidationError("bad value").
		WithContext(map[string]interface{}{"field": "status"}).
		WithSuggestion("Use a valid status name").
		WithErr(fmt.Errorf("underlying"))

	if e.Context["field"] != "status" {
		t.Fatalf("expected field=status, got %v", e.Context["field"])
	}
	if e.Suggestion != "Use a valid status name" {
		t.Fatalf("unexpected suggestion: %s", e.Suggestion)
	}
	if e.Err == nil || e.Err.Error() != "underlying" {
		t.Fatalf("unexpected wrapped error: %v", e.Err)
	}
}
