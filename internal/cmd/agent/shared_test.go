package agent

import (
	"testing"

	"context"
	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/auth"
	"net/http"
)

func blockerLink(key, statusCategoryKey string) api.IssueLink {
	return api.IssueLink{
		Type: &api.IssueLinkType{Name: "Blocks", Inward: "is blocked by", Outward: "blocks"},
		InwardIssue: &api.LinkedIssue{
			Key: key,
			Fields: &api.LinkedIssueFields{
				Status: &api.Status{
					Name:           "Some Status",
					StatusCategory: &api.StatusCategory{Key: statusCategoryKey},
				},
			},
		},
	}
}

func relatesLink(key string) api.IssueLink {
	return api.IssueLink{
		Type:         &api.IssueLinkType{Name: "Relates", Inward: "relates to", Outward: "relates to"},
		OutwardIssue: &api.LinkedIssue{Key: key},
	}
}

func TestIsBlocked(t *testing.T) {
	tests := []struct {
		name  string
		links []api.IssueLink
		want  bool
	}{
		{
			name:  "no links",
			links: nil,
			want:  false,
		},
		{
			name:  "blocked by open issue",
			links: []api.IssueLink{blockerLink("PROJ-1", "new")},
			want:  true,
		},
		{
			name:  "blocked by in-progress issue",
			links: []api.IssueLink{blockerLink("PROJ-1", "indeterminate")},
			want:  true,
		},
		{
			name:  "blocker is done",
			links: []api.IssueLink{blockerLink("PROJ-1", "done")},
			want:  false,
		},
		{
			name:  "relates link does not block",
			links: []api.IssueLink{relatesLink("PROJ-2")},
			want:  false,
		},
		{
			name: "mixed: one resolved, one unresolved",
			links: []api.IssueLink{
				blockerLink("PROJ-1", "done"),
				blockerLink("PROJ-2", "new"),
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := &api.Issue{
				Key:    "PROJ-10",
				Fields: api.IssueFields{IssueLinks: tt.links},
			}
			got := IsBlocked(issue)
			if got != tt.want {
				t.Errorf("IsBlocked() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindBlockers(t *testing.T) {
	issue := &api.Issue{
		Key: "PROJ-10",
		Fields: api.IssueFields{
			IssueLinks: []api.IssueLink{
				blockerLink("PROJ-1", "done"),
				blockerLink("PROJ-2", "new"),
				blockerLink("PROJ-3", "indeterminate"),
				relatesLink("PROJ-4"),
			},
		},
	}

	blockers := FindBlockers(issue)
	if len(blockers) != 2 {
		t.Fatalf("FindBlockers() returned %d blockers, want 2", len(blockers))
	}

	keys := map[string]bool{}
	for _, b := range blockers {
		keys[b.Key] = true
	}
	if !keys["PROJ-2"] || !keys["PROJ-3"] {
		t.Errorf("expected PROJ-2 and PROJ-3, got %v", keys)
	}
}

func TestFindTransitionByCategory(t *testing.T) {
	transitions := []api.Transition{
		{ID: "1", Name: "To Do", To: &api.Status{Name: "To Do", StatusCategory: &api.StatusCategory{Key: "new"}}},
		{ID: "2", Name: "Start Progress", To: &api.Status{Name: "In Progress", StatusCategory: &api.StatusCategory{Key: "indeterminate"}}},
		{ID: "3", Name: "Done", To: &api.Status{Name: "Done", StatusCategory: &api.StatusCategory{Key: "done"}}},
	}

	t.Run("find indeterminate", func(t *testing.T) {
		tr, err := FindTransitionByCategory(transitions, "indeterminate")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tr.ID != "2" {
			t.Errorf("expected transition ID 2, got %s", tr.ID)
		}
	})

	t.Run("find done", func(t *testing.T) {
		tr, err := FindTransitionByCategory(transitions, "done")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tr.ID != "3" {
			t.Errorf("expected transition ID 3, got %s", tr.ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := FindTransitionByCategory(transitions, "nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent category")
		}
	})
}

func TestMapPriorityRank(t *testing.T) {
	tests := []struct {
		name     string
		priority *api.Priority
		want     int
	}{
		{"nil priority", nil, 2},
		{"Highest", &api.Priority{Name: "Highest"}, 0},
		{"High", &api.Priority{Name: "High"}, 1},
		{"Medium", &api.Priority{Name: "Medium"}, 2},
		{"Low", &api.Priority{Name: "Low"}, 3},
		{"Lowest", &api.Priority{Name: "Lowest"}, 4},
		{"unknown", &api.Priority{Name: "Custom"}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapPriorityRank(tt.priority)
			if got != tt.want {
				t.Errorf("MapPriorityRank() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSortByPriorityThenCreated(t *testing.T) {
	issues := []api.Issue{
		{Key: "A", Fields: api.IssueFields{Priority: &api.Priority{Name: "Low"}, Created: "2026-01-03"}},
		{Key: "B", Fields: api.IssueFields{Priority: &api.Priority{Name: "High"}, Created: "2026-01-02"}},
		{Key: "C", Fields: api.IssueFields{Priority: &api.Priority{Name: "High"}, Created: "2026-01-01"}},
		{Key: "D", Fields: api.IssueFields{Priority: &api.Priority{Name: "Highest"}, Created: "2026-01-04"}},
	}

	SortByPriorityThenCreated(issues)

	want := []string{"D", "C", "B", "A"}
	for i, w := range want {
		if issues[i].Key != w {
			t.Errorf("position %d: got %s, want %s", i, issues[i].Key, w)
		}
	}
}

// The sub-task type name differs between project styles — "Sub-task" in
// company-managed projects, "Subtask" in team-managed ones — so it has to come
// from the project rather than from a literal in the source.
func TestResolveIssueTypeName(t *testing.T) {
	tests := []struct {
		name        string
		types       []api.IssueTypeCreateMeta
		status      int
		wantSubtask bool
		fallback    string
		want        string
	}{
		{
			name: "company-managed names it Sub-task",
			types: []api.IssueTypeCreateMeta{
				{Name: "Task", Subtask: false},
				{Name: "Sub-task", Subtask: true},
			},
			status:      http.StatusOK,
			wantSubtask: true,
			fallback:    "Sub-task",
			want:        "Sub-task",
		},
		{
			name: "team-managed names it Subtask",
			types: []api.IssueTypeCreateMeta{
				{Name: "Task", Subtask: false},
				{Name: "Subtask", Subtask: true},
			},
			status:      http.StatusOK,
			wantSubtask: true,
			fallback:    "Sub-task",
			want:        "Subtask",
		},
		{
			name: "standard type",
			types: []api.IssueTypeCreateMeta{
				{Name: "Story", Subtask: false},
				{Name: "Subtask", Subtask: true},
			},
			status:      http.StatusOK,
			wantSubtask: false,
			fallback:    "Task",
			want:        "Story",
		},
		{
			name:        "unreadable metadata falls back",
			status:      http.StatusForbidden,
			wantSubtask: true,
			fallback:    "Sub-task",
			want:        "Sub-task",
		},
		{
			name:        "no matching type falls back",
			types:       []api.IssueTypeCreateMeta{{Name: "Task", Subtask: false}},
			status:      http.StatusOK,
			wantSubtask: true,
			fallback:    "Sub-task",
			want:        "Sub-task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, srv := newTestFactory(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.status != http.StatusOK {
					w.WriteHeader(tt.status)
					return
				}
				writeJSON(w, api.CreateMetaIssueTypes{IssueTypes: tt.types})
			}))

			creds := &auth.Credentials{Instance: "test.atlassian.net", User: "u", Token: "t"}
			client := api.NewClient(creds, api.WithBaseURL(srv.URL))

			got := ResolveIssueTypeName(context.Background(), client, "PROJ", tt.wantSubtask, tt.fallback)
			if got != tt.want {
				t.Errorf("ResolveIssueTypeName() = %q, want %q", got, tt.want)
			}
		})
	}
}
