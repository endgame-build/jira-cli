package shared

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// SprintJQLClause maps a --sprint flag value to a JQL clause.
// "active" → sprint in openSprints(), "future" → sprint in futureSprints(),
// any other non-empty string → sprint = "name".
// Returns "" for empty input.
func SprintJQLClause(sprint string) string {
	switch strings.ToLower(sprint) {
	case "active":
		return "sprint in openSprints()"
	case "future":
		return "sprint in futureSprints()"
	case "":
		return ""
	default:
		return fmt.Sprintf("sprint = %q", sprint)
	}
}

// SprintRemainingDays computes calendar days remaining from an ISO date string.
// The end date is treated as end-of-day UTC. Returns 0 on parse failure or
// when the date is in the past.
func SprintRemainingDays(endDate string) int {
	if endDate == "" {
		return 0
	}
	dateStr := endDate
	if len(dateStr) > 10 {
		dateStr = dateStr[:10]
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return 0
	}
	// End of day: sprint ends at the end of the calendar date.
	endOfDay := t.Add(24 * time.Hour)
	days := endOfDay.Sub(time.Now().UTC()).Hours() / 24
	remaining := int(math.Ceil(days))
	if remaining < 0 {
		return 0
	}
	return remaining
}

// TruncateDate extracts the date portion (YYYY-MM-DD) from an ISO datetime string.
func TruncateDate(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}
