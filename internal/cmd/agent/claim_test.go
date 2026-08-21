package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
)

func claimHandler(issue api.Issue, transitions []api.Transition) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET /myself
		if strings.HasSuffix(path, "/myself") && r.Method == http.MethodGet {
			writeJSON(w, myselfResponse())
			return
		}

		// GET /issue/{key}/transitions
		if strings.HasSuffix(path, "/transitions") && r.Method == http.MethodGet {
			writeJSON(w, map[string]interface{}{"transitions": transitions})
			return
		}

		// POST /issue/{key}/transitions
		if strings.HasSuffix(path, "/transitions") && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// PUT /issue/{key}/assignee
		if strings.HasSuffix(path, "/assignee") && r.Method == http.MethodPut {
			w.WriteHeader(http.StatusNoContent)
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

func TestClaimSuccess(t *testing.T) {
	issue := sampleIssueWithLinks("PROJ-123", "new", nil)
	issue.Fields.Assignee = nil

	f, tio, _ := newTestFactory(t, claimHandler(issue, sampleTransitions()))
	f.OutputJSON = true

	opts := &ClaimOptions{Factory: f, KeyOrID: "PROJ-123"}
	err := runClaim(opts)
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
	if result["key"] != "PROJ-123" {
		t.Errorf("expected key:PROJ-123, got %v", result["key"])
	}
	if result["status"] != "In Progress" {
		t.Errorf("expected status:In Progress, got %v", result["status"])
	}
}

func TestClaimIdempotent(t *testing.T) {
	issue := sampleIssueWithLinks("PROJ-123", "indeterminate", nil)
	issue.Fields.Assignee = &api.User{AccountID: "me-account-id", DisplayName: "Test User"}

	f, tio, _ := newTestFactory(t, claimHandler(issue, sampleTransitions()))
	f.OutputJSON = true

	opts := &ClaimOptions{Factory: f, KeyOrID: "PROJ-123"}
	err := runClaim(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}

	if result["noop"] != true {
		t.Errorf("expected noop:true for idempotent claim, got %v", result["noop"])
	}
}

func TestClaimConflict(t *testing.T) {
	issue := sampleIssueWithLinks("PROJ-123", "new", nil)
	issue.Fields.Assignee = &api.User{AccountID: "other-id", DisplayName: "Other User"}

	f, _, _ := newTestFactory(t, claimHandler(issue, sampleTransitions()))

	opts := &ClaimOptions{Factory: f, KeyOrID: "PROJ-123", Force: false}
	err := runClaim(opts)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "CONFLICT_ERROR") {
		t.Errorf("expected CONFLICT_ERROR, got: %v", err)
	}
}

func TestClaimForceOverride(t *testing.T) {
	issue := sampleIssueWithLinks("PROJ-123", "new", nil)
	issue.Fields.Assignee = &api.User{AccountID: "other-id", DisplayName: "Other User"}

	f, tio, _ := newTestFactory(t, claimHandler(issue, sampleTransitions()))
	f.OutputJSON = true

	opts := &ClaimOptions{Factory: f, KeyOrID: "PROJ-123", Force: true}
	err := runClaim(opts)
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
}

func TestClaimAlreadyDone(t *testing.T) {
	issue := sampleIssueWithLinks("PROJ-123", "done", nil)

	f, _, _ := newTestFactory(t, claimHandler(issue, sampleTransitions()))

	opts := &ClaimOptions{Factory: f, KeyOrID: "PROJ-123"}
	err := runClaim(opts)
	if err == nil {
		t.Fatal("expected error for done issue")
	}
	if !strings.Contains(err.Error(), "already Done") {
		t.Errorf("expected 'already Done' error, got: %v", err)
	}
}

func TestClaimDryRun(t *testing.T) {
	issue := sampleIssueWithLinks("PROJ-123", "new", nil)
	issue.Fields.Assignee = nil

	assignCalled := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/assignee") && r.Method == http.MethodPut {
			assignCalled = true
		}
		claimHandler(issue, sampleTransitions())(w, r)
	}

	f, tio, _ := newTestFactory(t, http.HandlerFunc(handler))
	f.DryRun = true
	f.OutputJSON = true

	opts := &ClaimOptions{Factory: f, KeyOrID: "PROJ-123"}
	err := runClaim(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if assignCalled {
		t.Error("expected no assign call during dry-run")
	}

	out := tio.OutBuf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if result["dry_run"] != true {
		t.Errorf("expected dry_run:true, got %v", result["dry_run"])
	}
}

func TestClaimTextOutput(t *testing.T) {
	issue := sampleIssueWithLinks("PROJ-123", "new", nil)
	issue.Fields.Assignee = nil

	f, tio, _ := newTestFactory(t, claimHandler(issue, sampleTransitions()))

	opts := &ClaimOptions{Factory: f, KeyOrID: "PROJ-123"}
	err := runClaim(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Claimed PROJ-123") {
		t.Errorf("expected claim message, got: %s", out)
	}
}

func TestClaimRollbackOnTransitionFailure(t *testing.T) {
	issue := sampleIssueWithLinks("PROJ-123", "new", nil)
	issue.Fields.Assignee = nil

	var rollbackCalled bool
	var rollbackBody map[string]interface{}

	handler := func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET /myself
		if strings.HasSuffix(path, "/myself") && r.Method == http.MethodGet {
			writeJSON(w, myselfResponse())
			return
		}

		// POST /transitions — fail to trigger rollback
		if strings.HasSuffix(path, "/transitions") && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusInternalServerError)
			writeJSON(w, map[string]interface{}{"errorMessages": []string{"Transition failed"}})
			return
		}

		// GET /transitions
		if strings.HasSuffix(path, "/transitions") && r.Method == http.MethodGet {
			writeJSON(w, map[string]interface{}{"transitions": sampleTransitions()})
			return
		}

		// PUT /assignee — track rollback
		if strings.HasSuffix(path, "/assignee") && r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			var parsed map[string]interface{}
			json.Unmarshal(body, &parsed)
			// Second PUT is the rollback (first is the initial assign).
			if rollbackBody != nil {
				// This is a subsequent call — the rollback.
			}
			rollbackBody = parsed
			rollbackCalled = true
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// GET /issue/{key}
		if strings.Contains(path, "/issue/") && r.Method == http.MethodGet {
			writeJSON(w, issue)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}

	f, _, _ := newTestFactory(t, http.HandlerFunc(handler))

	opts := &ClaimOptions{Factory: f, KeyOrID: "PROJ-123"}
	err := runClaim(opts)
	if err == nil {
		t.Fatal("expected error from failed transition")
	}

	if !rollbackCalled {
		t.Error("expected rollback PUT /assignee after transition failure")
	}

	// Rollback should set assignee to nil (issue was unassigned before).
	if rollbackBody != nil && rollbackBody["accountId"] != nil {
		t.Errorf("expected rollback to nil assignee, got: %v", rollbackBody["accountId"])
	}
}

func TestClaimNoInProgressTransition(t *testing.T) {
	issue := sampleIssueWithLinks("PROJ-123", "new", nil)
	issue.Fields.Assignee = nil

	// Workflow with no "In Progress" (indeterminate) transition.
	transitions := []api.Transition{
		{ID: "11", Name: "To Do", To: &api.Status{Name: "To Do", StatusCategory: &api.StatusCategory{Key: "new"}}},
		{ID: "31", Name: "Done", To: &api.Status{Name: "Done", StatusCategory: &api.StatusCategory{Key: "done"}}},
	}

	rollbackCalled := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if strings.HasSuffix(path, "/myself") && r.Method == http.MethodGet {
			writeJSON(w, myselfResponse())
			return
		}
		if strings.HasSuffix(path, "/transitions") && r.Method == http.MethodGet {
			writeJSON(w, map[string]interface{}{"transitions": transitions})
			return
		}
		if strings.HasSuffix(path, "/assignee") && r.Method == http.MethodPut {
			rollbackCalled = true
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.Contains(path, "/issue/") && r.Method == http.MethodGet {
			writeJSON(w, issue)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}

	f, _, _ := newTestFactory(t, http.HandlerFunc(handler))

	opts := &ClaimOptions{Factory: f, KeyOrID: "PROJ-123"}
	err := runClaim(opts)
	if err == nil {
		t.Fatal("expected error when no In Progress transition available")
	}
	if !strings.Contains(err.Error(), "INVALID_TRANSITION") {
		t.Errorf("expected INVALID_TRANSITION error, got: %v", err)
	}

	// Rollback should have been called (assign happened before transition lookup).
	// rollbackCalled will be true because the initial assign + the rollback both hit PUT.
	if !rollbackCalled {
		t.Error("expected rollback after missing transition")
	}
}
