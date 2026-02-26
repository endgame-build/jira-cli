package issue

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/endgameio/jira-cli/internal/api"
	"github.com/endgameio/jira-cli/internal/auth"
	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/factory"
	"github.com/endgameio/jira-cli/internal/iostreams"
)

// sampleTransitions returns a representative set of transitions for tests.
func sampleTransitions() []api.Transition {
	return []api.Transition{
		{
			ID:   "11",
			Name: "Start Progress",
			To:   &api.Status{ID: "3", Name: "In Progress", StatusCategory: &api.StatusCategory{Key: "indeterminate"}},
		},
		{
			ID:   "21",
			Name: "Done",
			To:   &api.Status{ID: "5", Name: "Done", StatusCategory: &api.StatusCategory{Key: "done"}},
		},
		{
			ID:   "31",
			Name: "Reopen",
			To:   &api.Status{ID: "1", Name: "To Do", StatusCategory: &api.StatusCategory{Key: "new"}},
		},
	}
}

// moveHandler returns an HTTP handler for move-related endpoints.
func moveHandler(issue api.Issue, transitions []api.Transition, transitionErr int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET /issue/{key}/transitions
		if strings.HasSuffix(path, "/transitions") && r.Method == http.MethodGet {
			resp := struct {
				Transitions []api.Transition `json:"transitions"`
			}{Transitions: transitions}
			json.NewEncoder(w).Encode(resp)
			return
		}

		// POST /issue/{key}/transitions
		if strings.HasSuffix(path, "/transitions") && r.Method == http.MethodPost {
			if transitionErr != 0 {
				w.WriteHeader(transitionErr)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"errorMessages": []string{"Transition not allowed"},
				})
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// GET /issue/{key} (for fetching current status)
		if strings.HasPrefix(path, "/issue/") && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(issue)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

// newTestMoveFactory creates a Factory wired to a move-specific httptest server.
func newTestMoveFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams, *httptest.Server) {
	t.Helper()

	tio := iostreams.Test()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	creds := &auth.Credentials{
		Instance: "test.atlassian.net",
		User:     "test@example.com",
		Token:    "test-token",
	}
	client := api.NewClient(creds, api.WithBaseURL(srv.URL))

	f := factory.NewTestFactory(tio.IOStreams, nil, client)
	return f, tio, srv
}

func TestMoveExactMatch(t *testing.T) {
	issue := sampleIssue()
	transitions := sampleTransitions()

	f, tio, _ := newTestMoveFactory(t, moveHandler(issue, transitions, 0))

	opts := &MoveOptions{
		Factory:      f,
		KeyOrID:      "PROJ-123",
		TargetStatus: "Done",
	}

	err := runMove(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Moved PROJ-123 to Done") {
		t.Errorf("expected success message, got: %s", out)
	}
}

func TestMoveCaseInsensitive(t *testing.T) {
	issue := sampleIssue()
	transitions := sampleTransitions()

	f, tio, _ := newTestMoveFactory(t, moveHandler(issue, transitions, 0))

	opts := &MoveOptions{
		Factory:      f,
		KeyOrID:      "PROJ-123",
		TargetStatus: "done",
	}

	err := runMove(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Moved PROJ-123 to Done") {
		t.Errorf("expected success message, got: %s", out)
	}
}

func TestMoveSubstringMatch(t *testing.T) {
	issue := sampleIssue()
	transitions := sampleTransitions()

	f, tio, _ := newTestMoveFactory(t, moveHandler(issue, transitions, 0))

	opts := &MoveOptions{
		Factory:      f,
		KeyOrID:      "PROJ-123",
		TargetStatus: "progress",
	}

	err := runMove(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Moved PROJ-123 to In Progress") {
		t.Errorf("expected success message with 'In Progress', got: %s", out)
	}
}

func TestMoveAmbiguousMatch(t *testing.T) {
	issue := sampleIssue()
	// Create transitions where "o" matches multiple targets: "To Do" and "Done".
	transitions := []api.Transition{
		{
			ID:   "11",
			Name: "Move to To Do",
			To:   &api.Status{ID: "1", Name: "To Do"},
		},
		{
			ID:   "21",
			Name: "Move to Done",
			To:   &api.Status{ID: "5", Name: "Done"},
		},
	}

	f, _, _ := newTestMoveFactory(t, moveHandler(issue, transitions, 0))

	opts := &MoveOptions{
		Factory:      f,
		KeyOrID:      "PROJ-123",
		TargetStatus: "o",
	}

	err := runMove(opts)
	if err == nil {
		t.Fatal("expected error for ambiguous match")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T", err)
	}
	if cliErr.Code != clierrors.INVALID_TRANSITION {
		t.Errorf("expected INVALID_TRANSITION, got: %s", cliErr.Code)
	}
	if !strings.Contains(cliErr.Message, "Ambiguous") {
		t.Errorf("expected 'Ambiguous' in message, got: %s", cliErr.Message)
	}
	if cliErr.Context["available"] == nil {
		t.Error("expected 'available' in context")
	}
}

func TestMoveNoMatch(t *testing.T) {
	issue := sampleIssue()
	transitions := sampleTransitions()

	f, _, _ := newTestMoveFactory(t, moveHandler(issue, transitions, 0))

	opts := &MoveOptions{
		Factory:      f,
		KeyOrID:      "PROJ-123",
		TargetStatus: "Nonexistent",
	}

	err := runMove(opts)
	if err == nil {
		t.Fatal("expected error for no match")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T", err)
	}
	if cliErr.Code != clierrors.INVALID_TRANSITION {
		t.Errorf("expected INVALID_TRANSITION, got: %s", cliErr.Code)
	}
	if !strings.Contains(cliErr.Message, "No transition found") {
		t.Errorf("expected 'No transition found' in message, got: %s", cliErr.Message)
	}
}

func TestMove404(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Issue Does Not Exist"},
		})
	}

	f, _, _ := newTestMoveFactory(t, http.HandlerFunc(handler))

	opts := &MoveOptions{
		Factory:      f,
		KeyOrID:      "PROJ-999",
		TargetStatus: "Done",
	}

	err := runMove(opts)
	if err == nil {
		t.Fatal("expected error for 404")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T", err)
	}
	if cliErr.Code != clierrors.NOT_FOUND {
		t.Errorf("expected NOT_FOUND, got: %s", cliErr.Code)
	}
}

func TestMove403(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"You do not have permission"},
		})
	}

	f, _, _ := newTestMoveFactory(t, http.HandlerFunc(handler))

	opts := &MoveOptions{
		Factory:      f,
		KeyOrID:      "PROJ-123",
		TargetStatus: "Done",
	}

	err := runMove(opts)
	if err == nil {
		t.Fatal("expected error for 403")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got: %T", err)
	}
	if cliErr.Code != clierrors.PERMISSION_DENIED {
		t.Errorf("expected PERMISSION_DENIED, got: %s", cliErr.Code)
	}
}

func TestMoveJSON(t *testing.T) {
	issue := sampleIssue()
	transitions := sampleTransitions()

	f, tio, _ := newTestMoveFactory(t, moveHandler(issue, transitions, 0))
	f.OutputJSON = true

	opts := &MoveOptions{
		Factory:      f,
		KeyOrID:      "PROJ-123",
		TargetStatus: "Done",
	}

	err := runMove(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}

	if result["ok"] != true {
		t.Errorf("expected ok:true, got: %v", result["ok"])
	}
	if result["key"] != "PROJ-123" {
		t.Errorf("expected key:PROJ-123, got: %v", result["key"])
	}
	if result["action"] != "moved" {
		t.Errorf("expected action:moved, got: %v", result["action"])
	}
	if result["from"] != "In Progress" {
		t.Errorf("expected from:'In Progress', got: %v", result["from"])
	}
	if result["to"] != "Done" {
		t.Errorf("expected to:Done, got: %v", result["to"])
	}
	if result["transition"] != "Done" {
		t.Errorf("expected transition:Done, got: %v", result["transition"])
	}
}

func TestMoveQuiet(t *testing.T) {
	issue := sampleIssue()
	transitions := sampleTransitions()

	f, tio, _ := newTestMoveFactory(t, moveHandler(issue, transitions, 0))
	f.Quiet = true

	opts := &MoveOptions{
		Factory:      f,
		KeyOrID:      "PROJ-123",
		TargetStatus: "Done",
	}

	err := runMove(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tio.OutBuf.Len() > 0 {
		t.Errorf("expected no output with --quiet, got: %s", tio.OutBuf.String())
	}
}

func TestMatchTransitionExactTakesPrecedenceOverSubstring(t *testing.T) {
	// "Done" exact matches first transition, even though "Done" is also a
	// substring of "Undone" in the second transition.
	transitions := []api.Transition{
		{
			ID:   "10",
			Name: "Close",
			To:   &api.Status{ID: "5", Name: "Done"},
		},
		{
			ID:   "20",
			Name: "Revert",
			To:   &api.Status{ID: "9", Name: "Undone"},
		},
	}

	matched, err := matchTransition(transitions, "Done")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matched.ID != "10" {
		t.Errorf("expected exact match (ID=10), got: %s", matched.ID)
	}
}

func TestMatchTransitionSortDeterminism(t *testing.T) {
	// Two transitions to the same status — sorted by ID, first one wins.
	transitions := []api.Transition{
		{
			ID:   "30",
			Name: "Close via B",
			To:   &api.Status{ID: "5", Name: "Done"},
		},
		{
			ID:   "10",
			Name: "Close via A",
			To:   &api.Status{ID: "5", Name: "Done"},
		},
	}

	// matchTransition is called after sorting in runMove; test the raw function
	// with pre-sorted input to ensure first-match semantics.
	matched, err := matchTransition(transitions, "Done")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should match the first in slice order (ID=30 since not sorted here).
	// The sorting happens in runMove, not matchTransition.
	if matched.ID != "30" {
		t.Errorf("expected first match (ID=30), got: %s", matched.ID)
	}
}
