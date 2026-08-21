package sprint

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSprintAdd_ExplicitSprint(t *testing.T) {
	var gotPath string
	var gotBody map[string]interface{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/sprint/") {
			gotPath = r.URL.Path
			json.NewDecoder(r.Body).Decode(&gotBody)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	f, tio, _ := newTestFactory(t, handler, nil)

	opts := &AddOptions{Factory: f, SprintID: 42, IssueKeys: []string{"PROJ-1", "PROJ-2"}}
	if err := runAdd(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasSuffix(gotPath, "/sprint/42/issue") {
		t.Errorf("posted to %q, want a /sprint/42/issue path", gotPath)
	}
	issues, _ := gotBody["issues"].([]interface{})
	if len(issues) != 2 || issues[0] != "PROJ-1" {
		t.Errorf("body issues = %v, want [PROJ-1 PROJ-2]", gotBody["issues"])
	}
	if out := tio.OutBuf.String(); !strings.Contains(out, "Added 2 issue(s)") {
		t.Errorf("output = %q", out)
	}
}

// Without --sprint the command resolves the project's active sprint, which
// must work on a team-managed board (type "simple") too.
func TestSprintAdd_ResolvesActiveSprint(t *testing.T) {
	posted := false

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/board":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{{"id": 1, "name": "SCRUM board", "type": "simple"}},
				"isLast": true,
			})
		case strings.HasSuffix(r.URL.Path, "/sprint") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]interface{}{
				"values": []map[string]interface{}{{"id": 7, "name": "Sprint 0", "state": "active"}},
				"isLast": true,
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/sprint/7/issue"):
			posted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	f, tio, _ := newTestFactory(t, handler, nil)

	opts := &AddOptions{Factory: f, Project: "PROJ", IssueKeys: []string{"PROJ-1"}}
	if err := runAdd(opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !posted {
		t.Error("no issues were posted to the resolved active sprint")
	}
	if out := tio.OutBuf.String(); !strings.Contains(out, "Sprint 0 (7)") {
		t.Errorf("output = %q, want the resolved sprint named", out)
	}
}

func TestSprintAdd_NoActiveSprint(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"values": []interface{}{}, "isLast": true})
	})

	f, _, _ := newTestFactory(t, handler, nil)

	err := runAdd(&AddOptions{Factory: f, Project: "PROJ", IssueKeys: []string{"PROJ-1"}})
	if err == nil {
		t.Fatal("expected an error when the project has no active sprint")
	}
	if !strings.Contains(err.Error(), "No active sprint") {
		t.Errorf("error = %v, want it to name the missing sprint", err)
	}
}

func TestSprintAdd_DryRunWritesNothing(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			t.Errorf("dry-run posted to %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	})

	f, tio, _ := newTestFactory(t, handler, nil)
	f.DryRun = true

	if err := runAdd(&AddOptions{Factory: f, SprintID: 42, IssueKeys: []string{"PROJ-1"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out := tio.OutBuf.String(); !strings.Contains(out, "42") {
		t.Errorf("dry-run output = %q", out)
	}
}
