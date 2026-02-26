package output

import (
	"fmt"
	"time"
)

// RelativeTime converts an ISO 8601 timestamp to a human-readable relative
// string such as "5 minutes ago" or "yesterday". If parsing fails, the raw
// input is returned unchanged (graceful degradation). Only used for table
// output — JSON always preserves the original ISO 8601 value.
func RelativeTime(iso8601 string) string {
	t, err := time.Parse(time.RFC3339, iso8601)
	if err != nil {
		// Try without timezone (some Jira responses use millisecond precision)
		t, err = time.Parse("2006-01-02T15:04:05.000-0700", iso8601)
		if err != nil {
			return iso8601
		}
	}

	return relativeTimeSince(time.Since(t))
}

func relativeTimeSince(d time.Duration) string {
	if d < 0 {
		d = -d
	}

	seconds := int(d.Seconds())
	minutes := int(d.Minutes())
	hours := int(d.Hours())
	days := hours / 24
	months := days / 30
	years := days / 365

	switch {
	case seconds < 60:
		return "just now"
	case minutes == 1:
		return "1 minute ago"
	case minutes < 60:
		return fmt.Sprintf("%d minutes ago", minutes)
	case hours == 1:
		return "1 hour ago"
	case hours < 24:
		return fmt.Sprintf("%d hours ago", hours)
	case days == 1:
		return "yesterday"
	case days < 30:
		return fmt.Sprintf("%d days ago", days)
	case months == 1:
		return "1 month ago"
	case months < 12:
		return fmt.Sprintf("%d months ago", months)
	case years == 1:
		return "1 year ago"
	default:
		return fmt.Sprintf("%d years ago", years)
	}
}
