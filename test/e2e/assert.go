//go:build e2e

package e2e

import (
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	indexTimeout  = 90 * time.Second
	indexInterval = 3 * time.Second
)

// Eventually retries cond until it returns true or the deadline passes.
//
// Any assertion that reads through JQL search must use this: Jira Cloud's
// search index lags writes by seconds, so a single read is a coin flip. Use it
// for the whole assertion, not just the mutation, and never replace it with a
// fixed sleep — a bounded retry still fails on a real regression.
func Eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(indexTimeout)
	for attempt := 1; ; attempt++ {
		if cond() {
			if attempt > 1 {
				t.Logf("%s: satisfied after %d attempt(s)", what, attempt)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %s waiting for %s (%d attempts)", indexTimeout, what, attempt)
		}
		time.Sleep(indexInterval)
	}
}

// RequireNoStderr asserts a command produced no warnings.
//
// Several commands write to stderr and still exit 0 — agent status when a
// sprint fetch fails, agent discover when the link or comment fails. Against a
// healthy sandbox any stderr output is a regression, so asserting emptiness
// turns an invisible degradation into a failure.
func RequireNoStderr(t *testing.T, r Result) {
	t.Helper()
	if strings.TrimSpace(r.Stderr) != "" {
		t.Errorf("expected empty stderr\n%s", r)
	}
}

// RequireStderrContains asserts a warning was emitted, for the cases where a
// degraded path must stay visible.
func RequireStderrContains(t *testing.T, r Result, want string) {
	t.Helper()
	if !strings.Contains(r.Stderr, want) {
		t.Errorf("stderr does not contain %q\n%s", want, r)
	}
}

// RequireEmptyStdout asserts a failing command wrote nothing to stdout.
func RequireEmptyStdout(t *testing.T, r Result) {
	t.Helper()
	if strings.TrimSpace(r.Stdout) != "" {
		t.Errorf("expected empty stdout\n%s", r)
	}
}

// ReadyKeys extracts the keys of a ready result, preserving order.
func ReadyKeys(items []ReadyItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Key
	}
	return out
}

// BlockedKeys extracts the keys of a blocked result, preserving order.
func BlockedKeys(items []BlockedItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Key
	}
	return out
}

// Filter keeps the elements matching keep.
func Filter[T any](in []T, keep func(T) bool) []T {
	out := make([]T, 0, len(in))
	for _, v := range in {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

// Contains reports whether needle is present.
func Contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

// SortedCopy returns a sorted copy, for set comparisons where order is not part
// of the contract.
func SortedCopy(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return out
}

// EqualSets reports whether two slices hold the same elements, ignoring order.
func EqualSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sa, sb := SortedCopy(a), SortedCopy(b)
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

// EqualOrdered reports whether two slices are identical, order included.
func EqualOrdered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
