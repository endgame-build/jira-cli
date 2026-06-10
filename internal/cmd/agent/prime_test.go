package agent

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
)

func primeHandler(statuses []api.IssueTypeStatuses, issueTypes []api.IssueTypeCreateMeta) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// GET /project/{key}/statuses
		if strings.HasSuffix(path, "/statuses") && r.Method == http.MethodGet {
			writeJSON(w, statuses)
			return
		}

		// GET /issue/createmeta/{key}/issuetypes
		if strings.Contains(path, "/createmeta/") && r.Method == http.MethodGet {
			writeJSON(w, api.CreateMetaIssueTypes{IssueTypes: issueTypes})
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}
}

func TestPrimeOutput(t *testing.T) {
	statuses := []api.IssueTypeStatuses{
		{
			Name: "Task",
			Statuses: []api.StatusDetail{
				{Name: "To Do", StatusCategory: &api.StatusCategory{Key: "new"}},
				{Name: "In Progress", StatusCategory: &api.StatusCategory{Key: "indeterminate"}},
				{Name: "Done", StatusCategory: &api.StatusCategory{Key: "done"}},
			},
		},
	}
	issueTypes := []api.IssueTypeCreateMeta{
		{Name: "Task", Subtask: false},
		{Name: "Bug", Subtask: false},
		{Name: "Sub-task", Subtask: true},
	}

	f, tio, _ := newTestFactory(t, primeHandler(statuses, issueTypes))

	opts := &PrimeOptions{Factory: f, Project: "PROJ"}
	err := runPrime(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()

	// Check key sections exist.
	checks := []string{
		"# Jira Agent Workflow Context",
		"## Rules",
		"## Core Commands",
		"## Session Protocol",
		"## Project: PROJ",
		"jira agent ready",
		"jira agent claim",
		"jira agent close",
		"jira agent discover",
		"To Do (new)",
		"In Progress (indeterminate)",
		"Done (done)",
		"Task",
		"Bug",
		"Sub-task (subtask)",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Errorf("expected output to contain %q", check)
		}
	}
}

func TestPrimeFullMode(t *testing.T) {
	statuses := []api.IssueTypeStatuses{}
	issueTypes := []api.IssueTypeCreateMeta{}

	f, tio, _ := newTestFactory(t, primeHandler(statuses, issueTypes))

	opts := &PrimeOptions{Factory: f, Project: "PROJ", Full: true}
	err := runPrime(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "## Extended Reference") {
		t.Error("expected Extended Reference section in full mode")
	}
	if !strings.Contains(out, "--assignee @me") {
		t.Error("expected flag reference in full mode")
	}
}

func TestPrimeHandlesAPIErrors(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		// Both endpoints return errors.
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errorMessages": []string{"Permission denied"},
		})
	}

	f, tio, _ := newTestFactory(t, http.HandlerFunc(handler))

	opts := &PrimeOptions{Factory: f, Project: "PROJ"}
	err := runPrime(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v (prime should gracefully handle API errors)", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "# Jira Agent Workflow Context") {
		t.Error("expected output even when APIs fail")
	}

	errOut := tio.ErrBuf.String()
	if !strings.Contains(errOut, "Warning") {
		t.Error("expected warnings about API failures")
	}
}
