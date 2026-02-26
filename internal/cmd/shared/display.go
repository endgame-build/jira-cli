// Package shared provides common utilities used across multiple command packages.
package shared

import (
	"strings"

	"github.com/endgameio/jira-cli/internal/api"
)

// ColorHelper is the interface for colorizing output. Satisfied by iostreams.IOStreams.
type ColorHelper interface {
	Green(string) string
	Yellow(string) string
	Cyan(string) string
}

// FieldSet converts a string slice of field names to a set for O(1) lookup.
// Returns nil if no fields specified (meaning show all).
func FieldSet(fields []string) map[string]bool {
	if len(fields) == 0 {
		return nil
	}
	set := make(map[string]bool, len(fields))
	for _, f := range fields {
		set[strings.ToLower(strings.TrimSpace(f))] = true
	}
	return set
}

// ShowField returns true if the field should be displayed.
// If wantFields is nil (no filter), all fields are shown.
func ShowField(wantFields map[string]bool, name string) bool {
	if wantFields == nil {
		return true
	}
	return wantFields[name]
}

// StatusWithColor colorizes a status name based on its category.
func StatusWithColor(c ColorHelper, status *api.Status) string {
	if status == nil {
		return "Unknown"
	}
	name := status.Name
	if status.StatusCategory == nil {
		return name
	}
	switch status.StatusCategory.Key {
	case "done":
		return c.Green(name)
	case "indeterminate":
		return c.Cyan(name)
	case "new":
		return c.Yellow(name)
	default:
		return name
	}
}
