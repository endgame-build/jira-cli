package agent

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
)

func blockedHandler(issues []api.Issue) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// GET /myself — the credential probe an empty search triggers. Without
		// this arm the catch-all below answers 404, which the probe treats as
		// transient, so these tests would pass while no longer exercising the
		// path they claim to.
		if strings.HasSuffix(r.URL.Path, "/myself") {
			writeJSON(w, myselfResponse())
			return
		}
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/search/jql") {
			writeJSON(w, sampleSearchResponse(issues))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestBlockedFindsBlockedIssues(t *testing.T) {
	issues := []api.Issue{
		sampleIssueWithLinks("PROJ-1", "new", []api.IssueLink{
			blockerLink("PROJ-99", "indeterminate"),
		}),
		sampleIssueWithLinks("PROJ-2", "new", nil), // not blocked
		sampleIssueWithLinks("PROJ-3", "new", []api.IssueLink{
			blockerLink("PROJ-98", "new"),
			blockerLink("PROJ-97", "done"), // resolved
		}),
	}

	f, tio, _ := newTestFactory(t, blockedHandler(issues))
	f.OutputJSON = true

	opts := &BlockedOptions{Factory: f, Project: "PROJ", Limit: 50}
	err := runBlocked(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result struct {
		Data []blockedItem `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}

	if len(result.Data) != 2 {
		t.Fatalf("expected 2 blocked issues, got %d", len(result.Data))
	}

	keys := map[string]bool{}
	for _, d := range result.Data {
		keys[d.Key] = true
	}
	if !keys["PROJ-1"] || !keys["PROJ-3"] {
		t.Error("expected PROJ-1 and PROJ-3 in blocked results")
	}
	if keys["PROJ-2"] {
		t.Error("PROJ-2 should not be in blocked results")
	}
}

func TestBlockedEmpty(t *testing.T) {
	f, tio, _ := newTestFactory(t, blockedHandler(nil))

	opts := &BlockedOptions{Factory: f, Project: "PROJ", Limit: 50}
	err := runBlocked(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "No blocked issues found") {
		t.Errorf("expected empty message, got: %s", out)
	}
}

func TestBlockedShowsBlockerDetails(t *testing.T) {
	issues := []api.Issue{
		sampleIssueWithLinks("PROJ-5", "new", []api.IssueLink{
			blockerLink("PROJ-99", "indeterminate"),
		}),
	}

	f, tio, _ := newTestFactory(t, blockedHandler(issues))
	f.OutputJSON = true

	opts := &BlockedOptions{Factory: f, Project: "PROJ", Limit: 50}
	err := runBlocked(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result struct {
		Data []blockedItem `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}

	if len(result.Data) != 1 {
		t.Fatalf("expected 1 blocked issue, got %d", len(result.Data))
	}

	item := result.Data[0]
	if len(item.BlockedBy) != 1 {
		t.Fatalf("expected 1 blocker, got %d", len(item.BlockedBy))
	}
	if item.BlockedBy[0].Key != "PROJ-99" {
		t.Errorf("expected blocker PROJ-99, got %s", item.BlockedBy[0].Key)
	}
}
