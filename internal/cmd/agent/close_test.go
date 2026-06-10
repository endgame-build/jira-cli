package agent

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
)

func closeHandler(issue api.Issue, transitions []api.Transition) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// POST /issue/{key}/transitions
		if strings.HasSuffix(path, "/transitions") && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// GET /issue/{key}/transitions
		if strings.HasSuffix(path, "/transitions") && r.Method == http.MethodGet {
			writeJSON(w, map[string]interface{}{"transitions": transitions})
			return
		}

		// POST /issue/{key}/comment
		if strings.HasSuffix(path, "/comment") && r.Method == http.MethodPost {
			writeJSON(w, api.Comment{ID: "100"})
			return
		}

		// GET /issue/{key}
		if strings.Contains(path, "/issue/") && r.Method == http.MethodGet {
			writeJSON(w, issue)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func TestCloseSuccess(t *testing.T) {
	issue := sampleIssueWithLinks("PROJ-123", "indeterminate", nil)

	f, tio, _ := newTestFactory(t, closeHandler(issue, sampleTransitions()))
	f.OutputJSON = true

	opts := &CloseOptions{Factory: f, KeyOrID: "PROJ-123"}
	err := runClose(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}

	if result["ok"] != true {
		t.Errorf("expected ok:true, got %v", result["ok"])
	}
	if result["status"] != "Done" {
		t.Errorf("expected status:Done, got %v", result["status"])
	}
}

func TestCloseIdempotent(t *testing.T) {
	issue := sampleIssueWithLinks("PROJ-123", "done", nil)

	f, tio, _ := newTestFactory(t, closeHandler(issue, sampleTransitions()))
	f.OutputJSON = true

	opts := &CloseOptions{Factory: f, KeyOrID: "PROJ-123"}
	err := runClose(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if result["ok"] != true {
		t.Errorf("expected ok:true for idempotent close")
	}
}

func TestCloseWithReason(t *testing.T) {
	issue := sampleIssueWithLinks("PROJ-123", "indeterminate", nil)

	commentAdded := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/comment") && r.Method == http.MethodPost {
			commentAdded = true
			writeJSON(w, api.Comment{ID: "100"})
			return
		}
		closeHandler(issue, sampleTransitions())(w, r)
	}

	f, _, _ := newTestFactory(t, http.HandlerFunc(handler))

	opts := &CloseOptions{Factory: f, KeyOrID: "PROJ-123", Reason: "Implemented with tests"}
	err := runClose(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !commentAdded {
		t.Error("expected close reason comment to be added")
	}
}

func TestCloseSuggestNext(t *testing.T) {
	// Issue PROJ-123 blocks PROJ-200. When we close PROJ-123,
	// PROJ-200 should appear as unblocked.
	issue := api.Issue{
		Key: "PROJ-123",
		Fields: api.IssueFields{
			Summary: "Blocking issue",
			Status: &api.Status{
				Name:           "In Progress",
				StatusCategory: &api.StatusCategory{Key: "indeterminate"},
			},
			IssueLinks: []api.IssueLink{
				{
					Type:         &api.IssueLinkType{Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
					OutwardIssue: &api.LinkedIssue{Key: "PROJ-200"},
				},
			},
		},
	}

	// PROJ-200 has no other blockers.
	unblockedIssue := sampleIssueWithLinks("PROJ-200", "new", nil)

	handler := func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET /issue/PROJ-200
		if strings.Contains(path, "/issue/PROJ-200") && r.Method == http.MethodGet {
			writeJSON(w, unblockedIssue)
			return
		}

		closeHandler(issue, sampleTransitions())(w, r)
	}

	f, tio, _ := newTestFactory(t, http.HandlerFunc(handler))
	f.OutputJSON = true

	opts := &CloseOptions{Factory: f, KeyOrID: "PROJ-123", SuggestNext: true}
	err := runClose(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}

	unblocked, ok := result["unblocked"].([]interface{})
	if !ok {
		t.Fatal("expected unblocked array in output")
	}
	if len(unblocked) != 1 || unblocked[0] != "PROJ-200" {
		t.Errorf("expected [PROJ-200], got %v", unblocked)
	}
}

func TestCloseDryRun(t *testing.T) {
	issue := sampleIssueWithLinks("PROJ-123", "indeterminate", nil)

	transitionCalled := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/transitions") && r.Method == http.MethodPost {
			transitionCalled = true
		}
		closeHandler(issue, sampleTransitions())(w, r)
	}

	f, tio, _ := newTestFactory(t, http.HandlerFunc(handler))
	f.DryRun = true
	f.OutputJSON = true

	opts := &CloseOptions{Factory: f, KeyOrID: "PROJ-123", Reason: "test"}
	err := runClose(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if transitionCalled {
		t.Error("expected no transition call during dry-run")
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if result["dry_run"] != true {
		t.Errorf("expected dry_run:true")
	}
}

func TestCloseTextOutput(t *testing.T) {
	issue := sampleIssueWithLinks("PROJ-123", "indeterminate", nil)

	f, tio, _ := newTestFactory(t, closeHandler(issue, sampleTransitions()))

	opts := &CloseOptions{Factory: f, KeyOrID: "PROJ-123"}
	err := runClose(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Closed PROJ-123") {
		t.Errorf("expected close message, got: %s", out)
	}
}
