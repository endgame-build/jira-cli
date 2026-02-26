package api

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

// ──────────────────────────────────────────────
// Token-based pagination tests
// ──────────────────────────────────────────────

func TestFetchTokenPage_SinglePage(t *testing.T) {
	t.Parallel()

	fetcher := func(_ context.Context, token string, maxResults int) ([]string, string, bool, error) {
		return []string{"a", "b", "c"}, "", true, nil
	}

	items, meta, err := FetchTokenPage(context.Background(), 0, 10, fetcher, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d", len(items))
	}
	if meta.HasNextPage {
		t.Error("want HasNextPage=false for last page")
	}
	if meta.Total != nil {
		t.Error("want Total=nil for token-based pagination")
	}
	if meta.Offset != 0 {
		t.Errorf("want Offset=0, got %d", meta.Offset)
	}
	if meta.Limit != 10 {
		t.Errorf("want Limit=10, got %d", meta.Limit)
	}
}

func TestFetchTokenPage_MultiPage(t *testing.T) {
	t.Parallel()

	fetcher := func(_ context.Context, token string, maxResults int) ([]string, string, bool, error) {
		if token == "" {
			// First page: return full page, has more.
			items := make([]string, maxResults)
			for i := range items {
				items[i] = fmt.Sprintf("item-%d", i)
			}
			return items, "page2", false, nil
		}
		return nil, "", false, fmt.Errorf("should not fetch beyond first page")
	}

	items, meta, err := FetchTokenPage(context.Background(), 0, 5, fetcher, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 5 {
		t.Fatalf("want 5 items, got %d", len(items))
	}
	if !meta.HasNextPage {
		t.Error("want HasNextPage=true when not last page and full results")
	}
}

func TestFetchTokenPage_OffsetSkip(t *testing.T) {
	t.Parallel()

	// Simulate 3-item first page, then 2-item second page (last).
	callCount := 0
	fetcher := func(_ context.Context, token string, maxResults int) ([]string, string, bool, error) {
		callCount++
		switch {
		case callCount == 1:
			// Skip phase: return 3 items (offset=3 means skip exactly 3).
			return []string{"skip-1", "skip-2", "skip-3"}, "page2", false, nil
		case callCount == 2:
			// Real page: return the actual results.
			return []string{"result-1", "result-2"}, "", true, nil
		default:
			return nil, "", false, fmt.Errorf("too many calls")
		}
	}

	items, meta, err := FetchTokenPage(context.Background(), 3, 5, fetcher, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0] != "result-1" || items[1] != "result-2" {
		t.Errorf("want [result-1, result-2], got %v", items)
	}
	if meta.Offset != 3 {
		t.Errorf("want Offset=3, got %d", meta.Offset)
	}
	if meta.HasNextPage {
		t.Error("want HasNextPage=false for last page")
	}
	if callCount != 2 {
		t.Errorf("expected 2 fetcher calls (skip + real), got %d", callCount)
	}
}

func TestFetchTokenPage_OffsetExhausted(t *testing.T) {
	t.Parallel()

	// Only 2 items exist, but offset=10.
	fetcher := func(_ context.Context, token string, maxResults int) ([]string, string, bool, error) {
		return []string{"a", "b"}, "", true, nil
	}

	items, meta, err := FetchTokenPage(context.Background(), 10, 5, fetcher, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items != nil {
		t.Errorf("want nil items when offset exceeds total, got %v", items)
	}
	if meta.HasNextPage {
		t.Error("want HasNextPage=false when exhausted")
	}
	if meta.Offset != 10 {
		t.Errorf("want Offset=10, got %d", meta.Offset)
	}
}

func TestFetchTokenPage_OffsetWarning(t *testing.T) {
	t.Parallel()

	callCount := 0
	fetcher := func(_ context.Context, token string, maxResults int) ([]string, string, bool, error) {
		callCount++
		if callCount <= 11 {
			items := make([]string, maxResults)
			return items, fmt.Sprintf("page%d", callCount+1), false, nil
		}
		return []string{"result"}, "", true, nil
	}

	var buf bytes.Buffer
	items, _, err := FetchTokenPage(context.Background(), 1001, 5, fetcher, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	warning := buf.String()
	if !strings.Contains(warning, "1001") {
		t.Errorf("want warning mentioning offset 1001, got: %q", warning)
	}
	if !strings.Contains(warning, "Warning") {
		t.Errorf("want 'Warning' in output, got: %q", warning)
	}

	if len(items) != 1 {
		t.Errorf("want 1 result item, got %d", len(items))
	}
}

func TestFetchTokenPage_NoWarningUnder1000(t *testing.T) {
	t.Parallel()

	callCount := 0
	fetcher := func(_ context.Context, token string, maxResults int) ([]string, string, bool, error) {
		callCount++
		if callCount == 1 {
			items := make([]string, maxResults)
			return items, "page2", false, nil
		}
		return []string{"result"}, "", true, nil
	}

	var buf bytes.Buffer
	_, _, err := FetchTokenPage(context.Background(), 100, 5, fetcher, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if buf.Len() > 0 {
		t.Errorf("want no warning for offset=100, got: %q", buf.String())
	}
}

func TestFetchTokenPage_EmptyResults(t *testing.T) {
	t.Parallel()

	fetcher := func(_ context.Context, token string, maxResults int) ([]string, string, bool, error) {
		return nil, "", true, nil
	}

	items, meta, err := FetchTokenPage(context.Background(), 0, 10, fetcher, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("want 0 items, got %d", len(items))
	}
	if meta.HasNextPage {
		t.Error("want HasNextPage=false for empty results")
	}
}

func TestFetchTokenPage_FetcherError(t *testing.T) {
	t.Parallel()

	fetcher := func(_ context.Context, token string, maxResults int) ([]string, string, bool, error) {
		return nil, "", false, fmt.Errorf("network error")
	}

	_, _, err := FetchTokenPage(context.Background(), 0, 10, fetcher, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "network error") {
		t.Errorf("want 'network error', got: %v", err)
	}
}

func TestFetchTokenPage_FetcherErrorDuringSkip(t *testing.T) {
	t.Parallel()

	fetcher := func(_ context.Context, token string, maxResults int) ([]string, string, bool, error) {
		return nil, "", false, fmt.Errorf("skip phase error")
	}

	_, _, err := FetchTokenPage(context.Background(), 50, 10, fetcher, nil)
	if err == nil {
		t.Fatal("expected error during skip phase, got nil")
	}
}

func TestFetchTokenPage_DefaultLimit(t *testing.T) {
	t.Parallel()

	var capturedLimit int
	fetcher := func(_ context.Context, token string, maxResults int) ([]string, string, bool, error) {
		capturedLimit = maxResults
		return nil, "", true, nil
	}

	_, meta, err := FetchTokenPage(context.Background(), 0, 0, fetcher, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLimit != 50 {
		t.Errorf("want default limit=50, got %d", capturedLimit)
	}
	if meta.Limit != 50 {
		t.Errorf("want meta.Limit=50, got %d", meta.Limit)
	}
}

func TestFetchTokenPage_PartialPage(t *testing.T) {
	t.Parallel()

	// Returns fewer items than limit but isLast=false — still no next page
	// because we got fewer than requested.
	fetcher := func(_ context.Context, token string, maxResults int) ([]string, string, bool, error) {
		return []string{"a", "b"}, "next", false, nil
	}

	items, meta, err := FetchTokenPage(context.Background(), 0, 5, fetcher, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if meta.HasNextPage {
		t.Error("want HasNextPage=false when items < limit (partial page)")
	}
}

// ──────────────────────────────────────────────
// Offset-based pagination tests
// ──────────────────────────────────────────────

func TestFetchOffsetPage_FirstPage(t *testing.T) {
	t.Parallel()

	fetcher := func(_ context.Context, startAt, maxResults int) (*OffsetPageResult[string], error) {
		return &OffsetPageResult[string]{
			Items:   []string{"a", "b", "c"},
			StartAt: startAt,
			Total:   10,
		}, nil
	}

	items, meta, err := FetchOffsetPage(context.Background(), 0, 3, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d", len(items))
	}
	if !meta.HasNextPage {
		t.Error("want HasNextPage=true (3 of 10)")
	}
	if meta.Total == nil || *meta.Total != 10 {
		t.Errorf("want Total=10, got %v", meta.Total)
	}
	if meta.Offset != 0 {
		t.Errorf("want Offset=0, got %d", meta.Offset)
	}
	if meta.Limit != 3 {
		t.Errorf("want Limit=3, got %d", meta.Limit)
	}
}

func TestFetchOffsetPage_LastPage(t *testing.T) {
	t.Parallel()

	fetcher := func(_ context.Context, startAt, maxResults int) (*OffsetPageResult[string], error) {
		return &OffsetPageResult[string]{
			Items:   []string{"h", "i", "j"},
			StartAt: 7,
			Total:   10,
		}, nil
	}

	items, meta, err := FetchOffsetPage(context.Background(), 7, 5, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d", len(items))
	}
	if meta.HasNextPage {
		t.Error("want HasNextPage=false for last page (7+3 >= 10)")
	}
}

func TestFetchOffsetPage_EmptyResults(t *testing.T) {
	t.Parallel()

	fetcher := func(_ context.Context, startAt, maxResults int) (*OffsetPageResult[string], error) {
		return &OffsetPageResult[string]{
			Items:   nil,
			StartAt: 0,
			Total:   0,
		}, nil
	}

	items, meta, err := FetchOffsetPage(context.Background(), 0, 10, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("want 0 items, got %d", len(items))
	}
	if meta.HasNextPage {
		t.Error("want HasNextPage=false for empty results")
	}
	if meta.Total == nil || *meta.Total != 0 {
		t.Errorf("want Total=0, got %v", meta.Total)
	}
}

func TestFetchOffsetPage_FetcherError(t *testing.T) {
	t.Parallel()

	fetcher := func(_ context.Context, startAt, maxResults int) (*OffsetPageResult[string], error) {
		return nil, fmt.Errorf("server error")
	}

	_, _, err := FetchOffsetPage(context.Background(), 0, 10, fetcher)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFetchOffsetPage_DefaultLimit(t *testing.T) {
	t.Parallel()

	var capturedMax int
	fetcher := func(_ context.Context, startAt, maxResults int) (*OffsetPageResult[string], error) {
		capturedMax = maxResults
		return &OffsetPageResult[string]{
			Items: nil,
			Total: 0,
		}, nil
	}

	_, meta, err := FetchOffsetPage(context.Background(), 0, 0, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedMax != 50 {
		t.Errorf("want default maxResults=50, got %d", capturedMax)
	}
	if meta.Limit != 50 {
		t.Errorf("want meta.Limit=50, got %d", meta.Limit)
	}
}

// ──────────────────────────────────────────────
// Raw-array pagination tests
// ──────────────────────────────────────────────

func TestFetchRawArrayPage_FullPage(t *testing.T) {
	t.Parallel()

	fetcher := func(_ context.Context, startAt, maxResults int) ([]string, error) {
		items := make([]string, maxResults)
		for i := range items {
			items[i] = fmt.Sprintf("user-%d", startAt+i)
		}
		return items, nil
	}

	items, meta, err := FetchRawArrayPage(context.Background(), 0, 5, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 5 {
		t.Fatalf("want 5 items, got %d", len(items))
	}
	if !meta.HasNextPage {
		t.Error("want HasNextPage=true when len(items)==limit")
	}
	if meta.Total != nil {
		t.Error("want Total=nil for raw array pagination")
	}
}

func TestFetchRawArrayPage_PartialPage(t *testing.T) {
	t.Parallel()

	fetcher := func(_ context.Context, startAt, maxResults int) ([]string, error) {
		return []string{"a", "b"}, nil
	}

	items, meta, err := FetchRawArrayPage(context.Background(), 0, 5, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if meta.HasNextPage {
		t.Error("want HasNextPage=false when len(items) < limit")
	}
}

func TestFetchRawArrayPage_EmptyResults(t *testing.T) {
	t.Parallel()

	fetcher := func(_ context.Context, startAt, maxResults int) ([]string, error) {
		return nil, nil
	}

	items, meta, err := FetchRawArrayPage(context.Background(), 0, 10, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("want 0 items, got %d", len(items))
	}
	if meta.HasNextPage {
		t.Error("want HasNextPage=false for empty results")
	}
}

func TestFetchRawArrayPage_FetcherError(t *testing.T) {
	t.Parallel()

	fetcher := func(_ context.Context, startAt, maxResults int) ([]string, error) {
		return nil, fmt.Errorf("forbidden")
	}

	_, _, err := FetchRawArrayPage(context.Background(), 0, 10, fetcher)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFetchRawArrayPage_DefaultLimit(t *testing.T) {
	t.Parallel()

	var capturedMax int
	fetcher := func(_ context.Context, startAt, maxResults int) ([]string, error) {
		capturedMax = maxResults
		return nil, nil
	}

	_, meta, err := FetchRawArrayPage(context.Background(), 0, 0, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedMax != 50 {
		t.Errorf("want default maxResults=50, got %d", capturedMax)
	}
	if meta.Limit != 50 {
		t.Errorf("want meta.Limit=50, got %d", meta.Limit)
	}
}

func TestFetchRawArrayPage_WithOffset(t *testing.T) {
	t.Parallel()

	var capturedStartAt int
	fetcher := func(_ context.Context, startAt, maxResults int) ([]string, error) {
		capturedStartAt = startAt
		return []string{"result"}, nil
	}

	items, meta, err := FetchRawArrayPage(context.Background(), 25, 10, fetcher)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedStartAt != 25 {
		t.Errorf("want startAt=25, got %d", capturedStartAt)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if meta.Offset != 25 {
		t.Errorf("want meta.Offset=25, got %d", meta.Offset)
	}
}
