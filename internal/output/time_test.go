package output

import (
	"fmt"
	"testing"
	"time"
)

func TestRelativeTimeSince(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"zero seconds", 0, "just now"},
		{"30 seconds", 30 * time.Second, "just now"},
		{"59 seconds", 59 * time.Second, "just now"},
		{"1 minute", 1 * time.Minute, "1 minute ago"},
		{"90 seconds", 90 * time.Second, "1 minute ago"},
		{"5 minutes", 5 * time.Minute, "5 minutes ago"},
		{"59 minutes", 59 * time.Minute, "59 minutes ago"},
		{"1 hour", 1 * time.Hour, "1 hour ago"},
		{"90 minutes", 90 * time.Minute, "1 hour ago"},
		{"5 hours", 5 * time.Hour, "5 hours ago"},
		{"23 hours", 23 * time.Hour, "23 hours ago"},
		{"1 day", 24 * time.Hour, "yesterday"},
		{"36 hours", 36 * time.Hour, "yesterday"},
		{"2 days", 48 * time.Hour, "2 days ago"},
		{"15 days", 15 * 24 * time.Hour, "15 days ago"},
		{"29 days", 29 * 24 * time.Hour, "29 days ago"},
		{"30 days", 30 * 24 * time.Hour, "1 month ago"},
		{"45 days", 45 * 24 * time.Hour, "1 month ago"},
		{"60 days", 60 * 24 * time.Hour, "2 months ago"},
		{"11 months", 330 * 24 * time.Hour, "11 months ago"},
		{"12 months", 365 * 24 * time.Hour, "12 months ago"},
		{"18 months", 548 * 24 * time.Hour, "18 months ago"},
		{"2 years", 730 * 24 * time.Hour, "2 years ago"},
		{"5 years", 5 * 365 * 24 * time.Hour, "5 years ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := relativeTimeSince(tt.duration)
			if got != tt.want {
				t.Errorf("relativeTimeSince(%v) = %q, want %q", tt.duration, got, tt.want)
			}
		})
	}
}

func TestRelativeTime(t *testing.T) {
	// Test with a valid RFC3339 timestamp close to now.
	recent := time.Now().Add(-3 * time.Minute).Format(time.RFC3339)
	got := RelativeTime(recent)
	if got != "3 minutes ago" {
		t.Errorf("RelativeTime(3min ago) = %q, want %q", got, "3 minutes ago")
	}

	// Test with Jira millisecond format (numeric offset).
	jiraFmt := time.Now().Add(-2 * time.Hour).Format("2006-01-02T15:04:05.000-0700")
	got = RelativeTime(jiraFmt)
	if got != "2 hours ago" {
		t.Errorf("RelativeTime(jira format) = %q, want %q", got, "2 hours ago")
	}

	// Test with no-millis numeric offset (e.g. from older Jira versions).
	noMillisFmt := time.Now().Add(-10 * time.Minute).Format("2006-01-02T15:04:05-0700")
	got = RelativeTime(noMillisFmt)
	if got != "10 minutes ago" {
		t.Errorf("RelativeTime(no-millis offset) = %q, want %q", got, "10 minutes ago")
	}
}

func TestRelativeTimeParseFailure(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"not-a-date"},
		{""},
		{"2024-13-45T99:99:99Z"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("input=%q", tt.input), func(t *testing.T) {
			got := RelativeTime(tt.input)
			if got != tt.input {
				t.Errorf("RelativeTime(%q) = %q, want raw input back", tt.input, got)
			}
		})
	}
}
