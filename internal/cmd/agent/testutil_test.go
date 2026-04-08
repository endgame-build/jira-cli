package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/auth"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/iostreams"
)

// newTestFactory creates a pre-wired factory for agent command tests.
func newTestFactory(t *testing.T, handler http.Handler) (*factory.Factory, *iostreams.TestIOStreams, *httptest.Server) {
	t.Helper()

	tio := iostreams.Test()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	creds := &auth.Credentials{
		Instance: "test.atlassian.net",
		User:     "test@example.com",
		Token:    "test-token",
	}
	client := api.NewClient(creds, api.WithBaseURL(srv.URL), api.WithAgileBaseURL(srv.URL))

	f := factory.NewTestFactory(tio.IOStreams, nil, client)
	return f, tio, srv
}

// writeJSON is a test helper that writes JSON to a response writer.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// sampleIssueWithLinks creates a test issue with configurable links.
func sampleIssueWithLinks(key string, statusCategory string, links []api.IssueLink) api.Issue {
	return api.Issue{
		Key: key,
		Fields: api.IssueFields{
			Summary: "Test issue " + key,
			Status: &api.Status{
				Name: statusCategory,
				StatusCategory: &api.StatusCategory{
					Key: statusCategory,
				},
			},
			Priority:   &api.Priority{Name: "Medium"},
			IssueType:  &api.IssueType{Name: "Task"},
			Labels:     []string{"backend"},
			IssueLinks: links,
			Created:    "2026-01-01T00:00:00.000+0000",
			Updated:    "2026-01-01T00:00:00.000+0000",
		},
	}
}

// sampleSearchResponse creates a search results response.
func sampleSearchResponse(issues []api.Issue) api.SearchResults {
	return api.SearchResults{
		Issues: issues,
		IsLast: true,
	}
}

// sampleTransitions returns a standard set of transitions.
func sampleTransitions() []api.Transition {
	return []api.Transition{
		{ID: "11", Name: "To Do", To: &api.Status{Name: "To Do", StatusCategory: &api.StatusCategory{Key: "new"}}},
		{ID: "21", Name: "In Progress", To: &api.Status{Name: "In Progress", StatusCategory: &api.StatusCategory{Key: "indeterminate"}}},
		{ID: "31", Name: "Done", To: &api.Status{Name: "Done", StatusCategory: &api.StatusCategory{Key: "done"}}},
	}
}

// myselfResponse returns a standard /myself response.
func myselfResponse() api.User {
	return api.User{
		AccountID:   "me-account-id",
		DisplayName: "Test User",
		Active:      true,
	}
}
