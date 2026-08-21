package agent

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
)

func statusHandler(myWork []api.Issue) http.HandlerFunc {
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
			// Parse JQL to route different queries.
			body := map[string]interface{}{}
			json.NewDecoder(r.Body).Decode(&body)
			jql, _ := body["jql"].(string)

			if strings.Contains(jql, "assignee = currentUser()") {
				writeJSON(w, sampleSearchResponse(myWork))
				return
			}

			// Other queries return minimal results.
			writeJSON(w, sampleSearchResponse(nil))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestStatusJSON(t *testing.T) {
	myWork := []api.Issue{
		{
			Key: "PROJ-10",
			Fields: api.IssueFields{
				Summary:  "Implement feature",
				Status:   &api.Status{Name: "In Progress"},
				Priority: &api.Priority{Name: "High"},
			},
		},
	}

	f, tio, _ := newTestFactory(t, statusHandler(myWork))
	f.OutputJSON = true

	opts := &StatusOptions{Factory: f, Project: "PROJ"}
	err := runStatus(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result statusResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}

	if result.Project != "PROJ" {
		t.Errorf("expected project PROJ, got %s", result.Project)
	}
	if result.InProgressCount != 1 {
		t.Errorf("expected 1 in-progress, got %d", result.InProgressCount)
	}
	if len(result.MyWork) != 1 {
		t.Fatalf("expected 1 my_work item, got %d", len(result.MyWork))
	}
	if result.MyWork[0].Key != "PROJ-10" {
		t.Errorf("expected PROJ-10, got %s", result.MyWork[0].Key)
	}
}

func TestStatusTextOutput(t *testing.T) {
	f, tio, _ := newTestFactory(t, statusHandler(nil))

	opts := &StatusOptions{Factory: f, Project: "PROJ"}
	err := runStatus(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Project: PROJ") {
		t.Errorf("expected project header, got: %s", out)
	}
	if !strings.Contains(out, "Ready:") {
		t.Errorf("expected Ready count, got: %s", out)
	}
}
