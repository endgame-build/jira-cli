package sprint

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/auth"
	"github.com/endgame-build/jira-cli/internal/config"
	"github.com/endgame-build/jira-cli/internal/factory"
	"github.com/endgame-build/jira-cli/internal/iostreams"
)

func newTestFactory(t *testing.T, handler http.Handler, cfg config.Config) (*factory.Factory, *iostreams.TestIOStreams, *httptest.Server) {
	t.Helper()

	tio := iostreams.Test()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	creds := &auth.Credentials{
		Instance: "test.atlassian.net",
		User:     "test@example.com",
		Token:    "test-token",
	}
	client := api.NewClient(creds,
		api.WithBaseURL(srv.URL),
		api.WithAgileBaseURL(srv.URL),
	)

	f := factory.NewTestFactory(tio.IOStreams, cfg, client)
	return f, tio, srv
}

func TestSprintList_Text(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/board") && !strings.Contains(r.URL.Path, "/sprint"):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{
					{"id": 1, "name": "Scrum Board", "type": "scrum"},
				},
				"isLast": true,
			})
		case strings.Contains(r.URL.Path, "/sprint"):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{
					{"id": 10, "name": "Sprint 1", "state": "active", "startDate": "2026-04-01T00:00:00.000Z", "endDate": "2026-04-14T00:00:00.000Z", "goal": "Ship auth"},
					{"id": 11, "name": "Sprint 2", "state": "future", "goal": "Polish UI"},
				},
				"isLast": true,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	f, tio, _ := newTestFactory(t, handler, nil)
	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Sprint 1") {
		t.Errorf("expected output to contain 'Sprint 1', got:\n%s", out)
	}
	if !strings.Contains(out, "Sprint 2") {
		t.Errorf("expected output to contain 'Sprint 2', got:\n%s", out)
	}
	if !strings.Contains(out, "Ship auth") {
		t.Errorf("expected output to contain goal 'Ship auth', got:\n%s", out)
	}
}

func TestSprintList_JSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/board") && !strings.Contains(r.URL.Path, "/sprint"):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{
					{"id": 1, "name": "Board", "type": "scrum"},
				},
				"isLast": true,
			})
		case strings.Contains(r.URL.Path, "/sprint"):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{
					{"id": 10, "name": "Sprint 1", "state": "active"},
				},
				"isLast": true,
			})
		}
	})

	f, tio, _ := newTestFactory(t, handler, nil)
	f.OutputJSON = true
	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, `"name":"Sprint 1"`) && !strings.Contains(out, `"name": "Sprint 1"`) {
		t.Errorf("expected JSON to contain sprint name, got:\n%s", out)
	}
}

func TestSprintList_NoBoards(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"values": []interface{}{},
			"isLast": true,
		})
	})

	f, tio, _ := newTestFactory(t, handler, nil)
	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
	}

	err := runList(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "No sprints found") {
		t.Errorf("expected 'No sprints found', got:\n%s", out)
	}
}

func TestSprintList_StateFilter(t *testing.T) {
	var gotState string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/board") && !strings.Contains(r.URL.Path, "/sprint"):
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{
					{"id": 1, "name": "Board", "type": "scrum"},
				},
				"isLast": true,
			})
		case strings.Contains(r.URL.Path, "/sprint"):
			gotState = r.URL.Query().Get("state")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []interface{}{},
				"isLast": true,
			})
		}
	})

	f, _, _ := newTestFactory(t, handler, nil)
	opts := &ListOptions{
		Factory: f,
		Project: "PROJ",
		State:   "active",
	}

	_ = runList(opts)

	if gotState != "active" {
		t.Errorf("expected state=active query param, got %q", gotState)
	}
}

// A team-managed project's board reports its type as "simple" while carrying
// sprints exactly like a Scrum board. Filtering on "scrum" alone hid them.
func TestSprintList_TeamManagedBoard(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/board":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{
					{"id": 1, "name": "SCRUM board", "type": "simple"},
					{"id": 2, "name": "Ops", "type": "kanban"},
				},
				"isLast": true,
			})
		case strings.HasSuffix(r.URL.Path, "/sprint"):
			if !strings.Contains(r.URL.Path, "/board/1/") {
				t.Errorf("fetched sprints for %s; only the sprint-capable board should be used", r.URL.Path)
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{{"id": 7, "name": "Sprint 0", "state": "active"}},
				"isLast": true,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	f, tio, _ := newTestFactory(t, handler, nil)

	if err := runList(&ListOptions{Factory: f, Project: "PROJ", NoPager: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out := tio.OutBuf.String(); !strings.Contains(out, "Sprint 0") {
		t.Errorf("output = %q, want the team-managed board's sprint listed", out)
	}
}
