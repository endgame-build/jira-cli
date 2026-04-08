package sprint

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	clierrors "github.com/endgame-build/jira-cli/internal/errors"
)

func TestSprintActive_Found(t *testing.T) {
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
					{
						"id":        10,
						"name":      "Sprint 42",
						"state":     "active",
						"startDate": "2026-04-01T00:00:00.000Z",
						"endDate":   "2026-04-14T00:00:00.000Z",
						"goal":      "Complete auth refactor",
					},
				},
				"isLast": true,
			})
		}
	})

	f, tio, _ := newTestFactory(t, handler, nil)
	opts := &ActiveOptions{
		Factory: f,
		Project: "PROJ",
	}

	err := runActive(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, "Sprint 42") {
		t.Errorf("expected output to contain 'Sprint 42', got:\n%s", out)
	}
	if !strings.Contains(out, "Complete auth refactor") {
		t.Errorf("expected output to contain goal, got:\n%s", out)
	}
	if !strings.Contains(out, "2026-04-14") {
		t.Errorf("expected output to contain end date, got:\n%s", out)
	}
}

func TestSprintActive_JSON(t *testing.T) {
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
					{"id": 10, "name": "Sprint 42", "state": "active", "goal": "Ship it"},
				},
				"isLast": true,
			})
		}
	})

	f, tio, _ := newTestFactory(t, handler, nil)
	f.OutputJSON = true
	opts := &ActiveOptions{
		Factory: f,
		Project: "PROJ",
	}

	err := runActive(opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := tio.OutBuf.String()
	if !strings.Contains(out, `"name"`) {
		t.Errorf("expected JSON output, got:\n%s", out)
	}
}

func TestSprintActive_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"values": []interface{}{},
			"isLast": true,
		})
	})

	f, _, _ := newTestFactory(t, handler, nil)
	opts := &ActiveOptions{
		Factory: f,
		Project: "PROJ",
	}

	err := runActive(opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.NOT_FOUND {
		t.Errorf("expected NOT_FOUND code, got %s", cliErr.Code)
	}
}

func TestSprintActive_KanbanOnly(t *testing.T) {
	// GetActiveSprint queries with type=scrum, so a kanban-only project returns empty boards.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"values": []interface{}{},
			"isLast": true,
		})
	})

	f, _, _ := newTestFactory(t, handler, nil)
	opts := &ActiveOptions{
		Factory: f,
		Project: "PROJ",
	}

	err := runActive(opts)
	if err == nil {
		t.Fatal("expected error for kanban-only project")
	}

	var cliErr *clierrors.CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected CLIError, got %T: %v", err, err)
	}
	if cliErr.Code != clierrors.NOT_FOUND {
		t.Errorf("expected NOT_FOUND code, got %s", cliErr.Code)
	}
}
