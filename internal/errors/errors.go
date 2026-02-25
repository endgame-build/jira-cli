// Package errors defines the CLIError type used across all jira-cli commands.
// Every command returns errors (never os.Exit); main.go renders them.
package errors

import (
	"encoding/json"
	"fmt"
)

// ErrorCode is a machine-readable string identifying the error category.
type ErrorCode string

const (
	GENERAL_ERROR      ErrorCode = "GENERAL_ERROR"
	AUTH_ERROR         ErrorCode = "AUTH_ERROR"
	VALIDATION_ERROR   ErrorCode = "VALIDATION_ERROR"
	NOT_FOUND          ErrorCode = "NOT_FOUND"
	PERMISSION_DENIED  ErrorCode = "PERMISSION_DENIED"
	RATE_LIMITED       ErrorCode = "RATE_LIMITED"
	INVALID_TRANSITION ErrorCode = "INVALID_TRANSITION"
	MISSING_FIELD      ErrorCode = "MISSING_FIELD"
	UNKNOWN_TYPE       ErrorCode = "UNKNOWN_TYPE"
	AMBIGUOUS_USER     ErrorCode = "AMBIGUOUS_USER"
	NETWORK_ERROR      ErrorCode = "NETWORK_ERROR"
	CONFLICT_ERROR     ErrorCode = "CONFLICT_ERROR"
)

// exitCodes maps error codes to process exit codes (0-8).
var exitCodes = map[ErrorCode]int{
	GENERAL_ERROR:      1,
	AUTH_ERROR:         2,
	VALIDATION_ERROR:   3,
	NOT_FOUND:          4,
	PERMISSION_DENIED:  5,
	RATE_LIMITED:       6,
	INVALID_TRANSITION: 3, // validation family
	MISSING_FIELD:      3, // validation family
	UNKNOWN_TYPE:       3, // validation family
	AMBIGUOUS_USER:     3, // validation family
	NETWORK_ERROR:      7,
	CONFLICT_ERROR:     8,
}

// CLIError is a structured error carrying machine-readable code, context, and
// an actionable suggestion for self-correction by humans and agents.
type CLIError struct {
	Code       ErrorCode              `json:"code"`
	Message    string                 `json:"message"`
	Context    map[string]interface{} `json:"context,omitempty"`
	Suggestion string                 `json:"suggestion,omitempty"`
	ExitCode   int                    `json:"-"`
	Err        error                  `json:"-"`
}

// Error implements the error interface.
func (e *CLIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap supports errors.Is and errors.As chains.
func (e *CLIError) Unwrap() error {
	return e.Err
}

// MarshalJSON produces {"error": {"code": "...", "message": "...", ...}}.
func (e *CLIError) MarshalJSON() ([]byte, error) {
	inner := struct {
		Code       ErrorCode              `json:"code"`
		Message    string                 `json:"message"`
		Context    map[string]interface{} `json:"context,omitempty"`
		Suggestion string                 `json:"suggestion,omitempty"`
	}{
		Code:       e.Code,
		Message:    e.Message,
		Context:    e.Context,
		Suggestion: e.Suggestion,
	}
	return json.Marshal(struct {
		Error interface{} `json:"error"`
	}{Error: inner})
}

// newCLIError creates a CLIError with the given code and message.
func newCLIError(code ErrorCode, message string) *CLIError {
	return &CLIError{
		Code:     code,
		Message:  message,
		ExitCode: exitCodes[code],
	}
}

// WithContext returns the error with the given context map.
func (e *CLIError) WithContext(ctx map[string]interface{}) *CLIError {
	e.Context = ctx
	return e
}

// WithSuggestion returns the error with an actionable suggestion.
func (e *CLIError) WithSuggestion(s string) *CLIError {
	e.Suggestion = s
	return e
}

// WithErr wraps an underlying error for unwrapping.
func (e *CLIError) WithErr(err error) *CLIError {
	e.Err = err
	return e
}

// --- Constructors ---

// NewGeneralError creates a GENERAL_ERROR (exit 1).
func NewGeneralError(message string) *CLIError {
	return newCLIError(GENERAL_ERROR, message)
}

// NewAuthError creates an AUTH_ERROR (exit 2).
func NewAuthError(message string) *CLIError {
	return newCLIError(AUTH_ERROR, message)
}

// NewValidationError creates a VALIDATION_ERROR (exit 3).
func NewValidationError(message string) *CLIError {
	return newCLIError(VALIDATION_ERROR, message)
}

// NewNotFoundError creates a NOT_FOUND (exit 4) with optional key context.
func NewNotFoundError(message string, key string) *CLIError {
	e := newCLIError(NOT_FOUND, message)
	if key != "" {
		e.Context = map[string]interface{}{"key": key}
	}
	return e
}

// NewPermissionError creates a PERMISSION_DENIED (exit 5).
func NewPermissionError(message string) *CLIError {
	return newCLIError(PERMISSION_DENIED, message)
}

// NewRateLimitError creates a RATE_LIMITED (exit 6).
func NewRateLimitError(message string) *CLIError {
	return newCLIError(RATE_LIMITED, message)
}

// NewTransitionError creates an INVALID_TRANSITION (exit 3) with available transitions.
func NewTransitionError(message string, available []map[string]interface{}) *CLIError {
	e := newCLIError(INVALID_TRANSITION, message)
	e.Context = map[string]interface{}{"available": available}
	return e
}

// NewMissingFieldError creates a MISSING_FIELD (exit 3) with field and type context.
func NewMissingFieldError(field, issueType string) *CLIError {
	e := newCLIError(MISSING_FIELD, fmt.Sprintf("Missing required field: %s", field))
	e.Context = map[string]interface{}{"field": field, "type": issueType}
	return e
}

// NewUnknownTypeError creates an UNKNOWN_TYPE (exit 3).
func NewUnknownTypeError(message string) *CLIError {
	return newCLIError(UNKNOWN_TYPE, message)
}

// NewAmbiguousUserError creates an AMBIGUOUS_USER (exit 3) with match candidates.
func NewAmbiguousUserError(message string, matches []map[string]interface{}) *CLIError {
	e := newCLIError(AMBIGUOUS_USER, message)
	e.Context = map[string]interface{}{"matches": matches}
	return e
}

// NewNetworkError creates a NETWORK_ERROR (exit 7) with instance context.
func NewNetworkError(message string, instance string) *CLIError {
	e := newCLIError(NETWORK_ERROR, message)
	e.Context = map[string]interface{}{"instance": instance}
	return e
}

// NewConflictError creates a CONFLICT_ERROR (exit 8).
func NewConflictError(message string) *CLIError {
	return newCLIError(CONFLICT_ERROR, message)
}
