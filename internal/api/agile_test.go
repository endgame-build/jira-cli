package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/endgame-build/jira-cli/internal/auth"
)

// newAgileTestClient creates a Client whose agile base URL points at the test server.
func newAgileTestClient(t *testing.T, serverURL string) *Client {
	t.Helper()
	creds := &auth.Credentials{
		Instance: "test.atlassian.net",
		User:     "user@example.com",
		Token:    "test-api-token",
	}
	return NewClient(creds,
		withBaseURL(serverURL+"/rest/api/3"),
		WithAgileBaseURL(serverURL+"/rest/agile/1.0"),
	)
}

func TestGetBoardsForProject(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}

		// Return two pages of boards.
		if callCount == 1 {
			json.NewEncoder(w).Encode(boardPage{
				Values: []Board{
					{ID: 1, Name: "Scrum Board", Type: "scrum"},
					{ID: 2, Name: "Kanban Board", Type: "kanban"},
				},
				StartAt: 0,
				Total:   3,
				IsLast:  false,
			})
			return
		}
		json.NewEncoder(w).Encode(boardPage{
			Values: []Board{
				{ID: 3, Name: "Second Scrum", Type: "scrum"},
			},
			StartAt: 2,
			Total:   3,
			IsLast:  true,
		})
	}))
	defer srv.Close()

	client := newAgileTestClient(t, srv.URL)
	boards, err := client.GetBoardsForProject(context.Background(), "PROJ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(boards) != 3 {
		t.Fatalf("expected 3 boards, got %d", len(boards))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls (pagination), got %d", callCount)
	}
	if boards[0].Name != "Scrum Board" {
		t.Errorf("expected first board 'Scrum Board', got %q", boards[0].Name)
	}
}

func TestGetBoardsForProject_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(boardPage{
			Values: []Board{},
			IsLast: true,
		})
	}))
	defer srv.Close()

	client := newAgileTestClient(t, srv.URL)
	boards, err := client.GetBoardsForProject(context.Background(), "PROJ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(boards) != 0 {
		t.Fatalf("expected 0 boards, got %d", len(boards))
	}
}

func TestGetSprintsForBoard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify state param is passed through.
		if got := r.URL.Query().Get("state"); got != "active" {
			t.Errorf("expected state=active, got %q", got)
		}
		json.NewEncoder(w).Encode(sprintPage{
			Values: []Sprint{
				{ID: 10, Name: "Sprint 1", State: "active", Goal: "Ship auth"},
			},
			IsLast: true,
		})
	}))
	defer srv.Close()

	client := newAgileTestClient(t, srv.URL)
	sprints, err := client.GetSprintsForBoard(context.Background(), 42, "active")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sprints) != 1 {
		t.Fatalf("expected 1 sprint, got %d", len(sprints))
	}
	if sprints[0].Name != "Sprint 1" {
		t.Errorf("expected sprint name 'Sprint 1', got %q", sprints[0].Name)
	}
}

func TestGetActiveSprint(t *testing.T) {
	tests := []struct {
		name     string
		boards   []Board // scrum boards returned (type=scrum filter applied server-side)
		sprints  []Sprint
		wantNil  bool
		wantName string
	}{
		{
			name:    "no scrum boards",
			boards:  []Board{},
			wantNil: true,
		},
		{
			name:    "scrum board no active sprint",
			boards:  []Board{{ID: 1, Name: "Scrum", Type: "scrum"}},
			sprints: []Sprint{},
			wantNil: true,
		},
		{
			name:     "scrum board with active sprint",
			boards:   []Board{{ID: 1, Name: "Scrum", Type: "scrum"}},
			sprints:  []Sprint{{ID: 10, Name: "Sprint 42", State: "active"}},
			wantNil:  false,
			wantName: "Sprint 42",
		},
		{
			name: "multiple scrum boards — uses first",
			boards: []Board{
				{ID: 1, Name: "Team A", Type: "scrum"},
				{ID: 2, Name: "Team B", Type: "scrum"},
			},
			sprints:  []Sprint{{ID: 10, Name: "Sprint 7", State: "active"}},
			wantNil:  false,
			wantName: "Sprint 7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/rest/agile/1.0/board":
					// Verify type=scrum filter is applied.
					if got := r.URL.Query().Get("type"); got != "scrum" {
						t.Errorf("expected type=scrum query param, got %q", got)
					}
					json.NewEncoder(w).Encode(boardPage{Values: tt.boards, IsLast: true})
				case len(r.URL.Path) > len("/rest/agile/1.0/board/") &&
					r.URL.Query().Get("state") == "active":
					json.NewEncoder(w).Encode(sprintPage{Values: tt.sprints, IsLast: true})
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL)
					w.WriteHeader(http.StatusNotFound)
					fmt.Fprintf(w, `{"errorMessages":["not found"]}`)
				}
			}))
			defer srv.Close()

			client := newAgileTestClient(t, srv.URL)
			sprint, err := client.GetActiveSprint(context.Background(), "PROJ")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantNil {
				if sprint != nil {
					t.Errorf("expected nil, got %+v", sprint)
				}
				return
			}
			if sprint == nil {
				t.Fatal("expected sprint, got nil")
			}
			if sprint.Name != tt.wantName {
				t.Errorf("expected name %q, got %q", tt.wantName, sprint.Name)
			}
		})
	}
}
