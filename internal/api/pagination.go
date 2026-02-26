package api

import (
	"context"
	"fmt"
	"io"

	"github.com/endgameio/jira-cli/internal/output"
)

// ──────────────────────────────────────────────
// Token-based pagination (POST /search/jql)
// ──────────────────────────────────────────────

// TokenPageFetcher fetches a single page using a nextPageToken.
// Returns the page items, the next-page token (empty string if last page),
// whether this is the last page, and any error.
type TokenPageFetcher[T any] func(ctx context.Context, token string, maxResults int) (items []T, nextPageToken string, isLast bool, err error)

// FetchTokenPage retrieves a single logical page via token-based pagination.
//
// offset:     number of leading results to skip (consume-and-discard).
// limit:      max results to return to the caller.
// fetcher:    callback that executes a single API request for one page.
// warnWriter: if non-nil, a warning is written when offset > 1000.
//
// Returns the items, pagination metadata, and any error.
// For token-based endpoints, PaginationMeta.Total is always nil.
func FetchTokenPage[T any](
	ctx context.Context,
	offset int,
	limit int,
	fetcher TokenPageFetcher[T],
	warnWriter io.Writer,
) ([]T, *output.PaginationMeta, error) {

	if limit <= 0 {
		limit = 50 // sensible default
	}

	if offset > 1000 && warnWriter != nil {
		fmt.Fprintf(warnWriter, "Warning: --offset %d requires fetching and discarding %d results (slow for large offsets)\n", offset, offset)
	}

	// Phase 1: skip `offset` results by consuming pages.
	skipped := 0
	token := ""
	for skipped < offset {
		need := offset - skipped
		pageSize := need
		if pageSize > 100 {
			pageSize = 100 // reasonable batch size for skip phase
		}

		items, nextToken, isLast, err := fetcher(ctx, token, pageSize)
		if err != nil {
			return nil, nil, err
		}

		skipped += len(items)
		token = nextToken

		if isLast {
			// Exhausted results before reaching offset — return empty.
			return nil, &output.PaginationMeta{
				Offset:      offset,
				Limit:       limit,
				Total:       nil,
				HasNextPage: false,
			}, nil
		}
	}

	// Phase 2: fetch the actual requested page.
	items, nextToken, isLast, err := fetcher(ctx, token, limit)
	if err != nil {
		return nil, nil, err
	}

	hasNext := !isLast && len(items) > 0
	// If we got fewer than limit, there's no next page regardless of isLast.
	if len(items) < limit {
		hasNext = false
	}

	// If the server said there's more but we need to verify with nextToken.
	if !isLast && nextToken != "" && len(items) == limit {
		hasNext = true
	}

	meta := &output.PaginationMeta{
		Offset:      offset,
		Limit:       limit,
		Total:       nil, // token-based: total unknown
		HasNextPage: hasNext,
	}

	return items, meta, nil
}

// ──────────────────────────────────────────────
// Offset-based pagination (startAt / maxResults)
// ──────────────────────────────────────────────

// OffsetPageResult holds the response from a single offset-based API call.
type OffsetPageResult[T any] struct {
	Items   []T
	StartAt int
	Total   int
}

// OffsetPageFetcher fetches a single page using startAt and maxResults.
type OffsetPageFetcher[T any] func(ctx context.Context, startAt, maxResults int) (*OffsetPageResult[T], error)

// FetchOffsetPage retrieves a single page via offset-based pagination.
//
// offset:     the startAt value to pass to the API.
// limit:      max results per page (maxResults).
// fetcher:    callback that executes a single API request.
//
// Returns the items, pagination metadata, and any error.
func FetchOffsetPage[T any](
	ctx context.Context,
	offset int,
	limit int,
	fetcher OffsetPageFetcher[T],
) ([]T, *output.PaginationMeta, error) {

	if limit <= 0 {
		limit = 50
	}

	result, err := fetcher(ctx, offset, limit)
	if err != nil {
		return nil, nil, err
	}

	total := result.Total
	hasNext := offset+len(result.Items) < total

	meta := &output.PaginationMeta{
		Offset:      offset,
		Limit:       limit,
		Total:       &total,
		HasNextPage: hasNext,
	}

	return result.Items, meta, nil
}

// ──────────────────────────────────────────────
// Raw-array pagination (endpoints returning []T)
// ──────────────────────────────────────────────

// RawArrayFetcher fetches a page from an endpoint returning a plain array.
// startAt and maxResults are passed as query parameters.
type RawArrayFetcher[T any] func(ctx context.Context, startAt, maxResults int) ([]T, error)

// FetchRawArrayPage retrieves a single page from an endpoint that returns
// a plain JSON array (e.g. GET /user/search).
//
// Heuristic: has_next_page is true if len(results) == maxResults.
// Total is always nil (not provided by the API).
func FetchRawArrayPage[T any](
	ctx context.Context,
	offset int,
	limit int,
	fetcher RawArrayFetcher[T],
) ([]T, *output.PaginationMeta, error) {

	if limit <= 0 {
		limit = 50
	}

	items, err := fetcher(ctx, offset, limit)
	if err != nil {
		return nil, nil, err
	}

	hasNext := len(items) == limit

	meta := &output.PaginationMeta{
		Offset:      offset,
		Limit:       limit,
		Total:       nil, // raw array: total unknown
		HasNextPage: hasNext,
	}

	return items, meta, nil
}
