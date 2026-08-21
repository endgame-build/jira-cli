//go:build e2e

package e2e

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// The response models below are restated here rather than imported from
// internal/cmd/agent, where they are unexported. Restating them makes this
// package an independent statement of the documented contract in
// docs/agent-sdlc-contracts.md: renaming a JSON field breaks these tests, which
// is the point.

// Pagination is the envelope that wraps list responses.
type Pagination struct {
	Offset      int  `json:"offset"`
	Limit       int  `json:"limit"`
	Total       *int `json:"total"`
	HasNextPage bool `json:"has_next_page"`
}

type listEnvelope[T any] struct {
	Data       []T         `json:"data"`
	Pagination *Pagination `json:"pagination"`
}

// ReadyItem is one entry of `agent ready`.
type ReadyItem struct {
	Key     string `json:"key"`
	Summary string `json:"summary"`
	Status  struct {
		Name     string `json:"name"`
		Category string `json:"category"`
	} `json:"status"`
	Priority struct {
		Name string `json:"name"`
		Rank int    `json:"rank"`
	} `json:"priority"`
	Type    string   `json:"type"`
	Labels  []string `json:"labels"`
	Created string   `json:"created"`
	Updated string   `json:"updated"`
	Parent  string   `json:"parent"`
}

// LinkedRef is one blocker inside a BlockedItem.
type LinkedRef struct {
	Key     string `json:"key"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
}

// BlockedItem is one entry of `agent blocked`.
type BlockedItem struct {
	Key       string      `json:"key"`
	Summary   string      `json:"summary"`
	Status    string      `json:"status"`
	BlockedBy []LinkedRef `json:"blocked_by"`
}

// SprintInfo is the sprint block embedded in `agent status`.
type SprintInfo struct {
	Name          string `json:"name"`
	Goal          string `json:"goal"`
	EndDate       string `json:"end_date"`
	RemainingDays int    `json:"remaining_days"`
}

// WorkItem is one entry of the my_work list in `agent status`.
type WorkItem struct {
	Key      string `json:"key"`
	Summary  string `json:"summary"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

// StatusDoc is the bare object emitted by `agent status` — no envelope.
type StatusDoc struct {
	Project         string      `json:"project"`
	Sprint          *SprintInfo `json:"sprint"`
	ReadyCount      int         `json:"ready_count"`
	InProgressCount int         `json:"in_progress_count"`
	BlockedCount    int         `json:"blocked_count"`
	DoneToday       int         `json:"done_today"`
	MyWork          []WorkItem  `json:"my_work"` // null when empty
}

// SprintDoc is the bare object emitted by `sprint active`.
type SprintDoc struct {
	Name          string `json:"name"`
	Goal          string `json:"goal"`
	State         string `json:"state"`
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
	RemainingDays int    `json:"remaining_days"`
	BoardID       int    `json:"board_id"`
}

// SprintListItem is one entry of `sprint list`.
type SprintListItem struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Goal      string `json:"goal"`
	BoardID   int    `json:"board_id"`
}

// MutationDoc is the flattened {"ok":true, ...} envelope shared by claim, close
// and discover. Fields absent from a given command decode to their zero value,
// which is why several assertions check the raw document instead.
type MutationDoc struct {
	OK             bool     `json:"ok"`
	Key            string   `json:"key"`
	Status         string   `json:"status"`
	PreviousStatus string   `json:"previous_status"`
	Assignee       string   `json:"assignee"`
	Noop           bool     `json:"noop"`
	Unblocked      []string `json:"unblocked"`
	Parent         string   `json:"parent"`
	Relationship   string   `json:"relationship"`
	Summary        string   `json:"summary"`
	Type           string   `json:"type"`
	Priority       string   `json:"priority"`
}

// DryRunDoc is the {"dry_run":true,...} envelope.
type DryRunDoc struct {
	DryRun     bool           `json:"dry_run"`
	Payload    map[string]any `json:"payload"`
	Validation string         `json:"validation"`
}

// ErrorDoc is the error written to STDERR in JSON mode, unwrapped from its
// {"error":{...}} container.
type ErrorDoc struct {
	Code       string         `json:"code"`
	Message    string         `json:"message"`
	Suggestion string         `json:"suggestion"`
	Context    map[string]any `json:"context"`
}

// DecodeList decodes the {"data":[...],"pagination":{...}} envelope.
//
// `agent ready` emits "data": [] for an empty result while `agent blocked` and
// `sprint list` emit "data": null for the same condition. Both land here as a
// non-nil empty slice so no caller has to branch on it; the cases that pin that
// inconsistency assert on the raw JSON instead.
func DecodeList[T any](t *testing.T, r Result) ([]T, *Pagination) {
	t.Helper()
	var env listEnvelope[T]
	decodeSingle(t, r, &env)
	if env.Data == nil {
		env.Data = []T{}
	}
	return env.Data, env.Pagination
}

// DecodeObject decodes a bare JSON object with no envelope — `agent status`
// and `sprint active`.
func DecodeObject[T any](t *testing.T, r Result) T {
	t.Helper()
	var v T
	decodeSingle(t, r, &v)
	return v
}

// DecodeDocs returns every JSON document written to stdout.
//
// `agent close --claim-next --json` writes two concatenated objects — the claim
// mutation, then the close mutation — so json.Unmarshal over the whole buffer
// fails with "invalid character '{' after top-level value".
func DecodeDocs(t *testing.T, r Result) []json.RawMessage {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(r.Stdout))
	var docs []json.RawMessage
	for {
		var raw json.RawMessage
		err := dec.Decode(&raw)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("stdout is not a stream of JSON documents: %v\n%s", err, r)
		}
		docs = append(docs, raw)
	}
	return docs
}

// DecodeDocAt decodes the i-th document of a multi-document stdout.
func DecodeDocAt[T any](t *testing.T, r Result, i int) T {
	t.Helper()
	docs := DecodeDocs(t, r)
	if i >= len(docs) {
		t.Fatalf("expected at least %d JSON document(s) on stdout, got %d\n%s", i+1, len(docs), r)
	}
	var v T
	if err := json.Unmarshal(docs[i], &v); err != nil {
		t.Fatalf("document %d: %v\n%s", i, err, r)
	}
	return v
}

// DecodeRaw decodes stdout into a generic map, for assertions about the presence
// or absence of a key rather than its value.
func DecodeRaw(t *testing.T, r Result) map[string]any {
	t.Helper()
	var m map[string]any
	decodeSingle(t, r, &m)
	return m
}

func decodeSingle(t *testing.T, r Result, out any) {
	t.Helper()
	docs := DecodeDocs(t, r)
	if len(docs) != 1 {
		t.Fatalf("expected exactly 1 JSON document on stdout, got %d\n%s", len(docs), r)
	}
	if err := json.Unmarshal(docs[0], out); err != nil {
		t.Fatalf("decode: %v\n%s", err, r)
	}
}

// DecodeError decodes the error document from stderr and asserts its code.
func DecodeError(t *testing.T, r Result, wantCode string) ErrorDoc {
	t.Helper()
	var wrapper struct {
		Error ErrorDoc `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(r.Stderr)), &wrapper); err != nil {
		t.Fatalf("stderr is not a JSON error document: %v\n%s", err, r)
	}
	if wrapper.Error.Code != wantCode {
		t.Errorf("error code = %q, want %q\n%s", wrapper.Error.Code, wantCode, r)
	}
	return wrapper.Error
}
