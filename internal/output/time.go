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
	// Jira Cloud uses "2006-01-02T15:04:05.000+0000" (millis, no colon in offset).
	// RFC3339 handles Z and colon-offsets; the second format handles Jira's style.
	// The third handles no-millis with Jira-style offset (rare but possible).
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05-0700",
	}
	var t time.Time
	var err error
	for _, layout := range formats {
		t, err = time.Parse(layout, iso8601)
		if err == nil {
			break
		}
	}
	if err != nil {
		return iso8601
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
	case months < 24:
		return fmt.Sprintf("%d months ago", months)
	case years < 2:
		return "1 year ago"
	default:
		return fmt.Sprintf("%d years ago", years)
	}
}
