package agent

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
)

func readyHandler(issues []api.Issue) http.HandlerFunc {
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

func TestReadyFiltersBlockedIssues(t *testing.T) {
	issues := []api.Issue{
		sampleIssueWithLinks("PROJ-1", "new", nil),
		sampleIssueWithLinks("PROJ-2", "new", []api.IssueLink{
			blockerLink("PROJ-99", "indeterminate"),
		}),
		sampleIssueWithLinks("PROJ-3", "new", nil),
	}

	f, tio, _ := newTestFactory(t, readyHandler(issues))
	f.OutputJSON = true

	opts := &ReadyOptions{
		Factory: f,
		Project: "PROJ",
		Limit:   10,
		Sort:    "priority",
	}

	err := runReady(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result struct {
		Data []struct {
			Key string `json:"key"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}

	if len(result.Data) != 2 {
		t.Fatalf("expected 2 ready issues, got %d", len(result.Data))
	}

	keys := map[string]bool{}
	for _, d := range result.Data {
		keys[d.Key] = true
	}
	if keys["PROJ-2"] {
		t.Error("PROJ-2 should be filtered out (blocked)")
	}
	if !keys["PROJ-1"] || !keys["PROJ-3"] {
		t.Error("PROJ-1 and PROJ-3 should be in results")
	}
}

func TestReadyEmptyQueue(t *testing.T) {
	f, tio, _ := newTestFactory(t, readyHandler(nil))
	f.OutputJSON = true

	opts := &ReadyOptions{
		Factory: f,
		Project: "PROJ",
		Limit:   10,
		Sort:    "priority",
	}

	err := runReady(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result struct {
		Data []interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}

	if len(result.Data) != 0 {
		t.Errorf("expected empty data, got %d items", len(result.Data))
	}
}

func TestReadyRespectsLimit(t *testing.T) {
	issues := make([]api.Issue, 20)
	for i := range issues {
		issues[i] = sampleIssueWithLinks("PROJ-"+string(rune('A'+i)), "new", nil)
	}

	f, tio, _ := newTestFactory(t, readyHandler(issues))
	f.OutputJSON = true

	opts := &ReadyOptions{
		Factory: f,
		Project: "PROJ",
		Limit:   5,
		Sort:    "priority",
	}

	err := runReady(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result struct {
		Data []interface{} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}

	if len(result.Data) != 5 {
		t.Errorf("expected 5 items (limit), got %d", len(result.Data))
	}
}

func TestReadyPriorityOrdering(t *testing.T) {
	issues := []api.Issue{
		{
			Key: "PROJ-LOW",
			Fields: api.IssueFields{
				Summary:   "Low priority",
				Status:    &api.Status{Name: "To Do", StatusCategory: &api.StatusCategory{Key: "new"}},
				Priority:  &api.Priority{Name: "Low"},
				IssueType: &api.IssueType{Name: "Task"},
				Created:   "2026-01-01T00:00:00.000+0000",
				Updated:   "2026-01-01T00:00:00.000+0000",
			},
		},
		{
			Key: "PROJ-HIGH",
			Fields: api.IssueFields{
				Summary:   "High priority",
				Status:    &api.Status{Name: "To Do", StatusCategory: &api.StatusCategory{Key: "new"}},
				Priority:  &api.Priority{Name: "High"},
				IssueType: &api.IssueType{Name: "Task"},
				Created:   "2026-01-02T00:00:00.000+0000",
				Updated:   "2026-01-02T00:00:00.000+0000",
			},
		},
	}

	f, tio, _ := newTestFactory(t, readyHandler(issues))
	f.OutputJSON = true

	opts := &ReadyOptions{
		Factory: f,
		Project: "PROJ",
		Limit:   10,
		Sort:    "priority",
	}

	err := runReady(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	var result struct {
		Data []struct {
			Key      string `json:"key"`
			Priority struct {
				Rank int `json:"rank"`
			} `json:"priority"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}

	if len(result.Data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Data))
	}

	if result.Data[0].Key != "PROJ-HIGH" {
		t.Errorf("expected PROJ-HIGH first (higher priority), got %s", result.Data[0].Key)
	}
}

func TestReadyTextEmptyOutput(t *testing.T) {
	f, tio, _ := newTestFactory(t, readyHandler(nil))

	opts := &ReadyOptions{
		Factory: f,
		Project: "PROJ",
		Limit:   10,
		Sort:    "priority",
	}

	err := runReady(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "No ready issues found") {
		t.Errorf("expected empty message, got: %s", out)
	}
}

// --sort used to be discarded: the JQL asked Jira for one order and a
// client-side re-sort immediately imposed another. Because truncation to
// --limit happens after that sort, the wrong order changed which issues came
// back, not just how they were arranged.
func TestReadyHonoursExplicitSort(t *testing.T) {
	// Server order is deliberately the opposite of priority order.
	serverOrder := []api.Issue{
		sampleIssueWithLinks("PROJ-LOW", "new", nil),
		sampleIssueWithLinks("PROJ-HIGH", "new", nil),
	}
	serverOrder[0].Fields.Priority = &api.Priority{Name: "Low"}
	serverOrder[0].Fields.Created = "2026-01-01T00:00:00.000+0000"
	serverOrder[1].Fields.Priority = &api.Priority{Name: "Highest"}
	serverOrder[1].Fields.Created = "2026-01-02T00:00:00.000+0000"

	tests := []struct {
		sort      string
		wantFirst string
		why       string
	}{
		{sort: "priority", wantFirst: "PROJ-HIGH", why: "the default re-sorts by priority"},
		{sort: "created", wantFirst: "PROJ-LOW", why: "an explicit sort keeps the order Jira returned"},
		{sort: "updated", wantFirst: "PROJ-LOW", why: "an explicit sort keeps the order Jira returned"},
	}

	for _, tt := range tests {
		t.Run(tt.sort, func(t *testing.T) {
			var gotJQL string
			f, tio, _ := newTestFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/myself") {
					writeJSON(w, myselfResponse())
					return
				}
				body := map[string]interface{}{}
				json.NewDecoder(r.Body).Decode(&body)
				gotJQL, _ = body["jql"].(string)
				writeJSON(w, sampleSearchResponse(serverOrder))
			}))
			f.OutputJSON = true

			opts := &ReadyOptions{Factory: f, Project: "PROJ", Limit: 10, Sort: tt.sort}
			if err := runReady(opts); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// The JQL must ask for the requested order in the first place.
			switch tt.sort {
			case "created":
				if !strings.Contains(gotJQL, "ORDER BY created ASC") {
					t.Errorf("JQL = %q, want it to order by created", gotJQL)
				}
			case "updated":
				if !strings.Contains(gotJQL, "ORDER BY updated DESC") {
					t.Errorf("JQL = %q, want it to order by updated", gotJQL)
				}
			}

			var result struct {
				Data []struct {
					Key string `json:"key"`
				} `json:"data"`
			}
			if err := json.Unmarshal(tio.OutBuf.Bytes(), &result); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(result.Data) == 0 {
				t.Fatal("no issues returned")
			}
			if result.Data[0].Key != tt.wantFirst {
				t.Errorf("first issue = %s, want %s — %s", result.Data[0].Key, tt.wantFirst, tt.why)
			}
		})
	}
}

// An unrecognised --sort used to fall through to the default silently.
func TestReadyRejectsUnknownSort(t *testing.T) {
	f, _, _ := newTestFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the sort guard should fire before any request")
	}))

	cmd := NewCmdReady(f)
	cmd.SetArgs([]string{"--project", "PROJ", "--sort", "nonsense"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for an unknown --sort value")
	}
}
