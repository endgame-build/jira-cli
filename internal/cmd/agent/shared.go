package agent

import (
	"context"
	"sort"
	"strings"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/cmd/shared"
	clierrors "github.com/endgame-build/jira-cli/internal/errors"
)

// Status category keys used by Jira Cloud. Commands match on these
// rather than status names for portability across project workflows.
const (
	CategoryToDo       = "new"
	CategoryInProgress = "indeterminate"
	CategoryDone       = "done"
)

// IsBlocked checks if an issue has unresolved "is blocked by" links.
func IsBlocked(issue *api.Issue) bool {
	for _, link := range issue.Fields.IssueLinks {
		if link.Type == nil {
			continue
		}
		if link.Type.Inward == "is blocked by" && link.InwardIssue != nil {
			if !isLinkedIssueDone(link.InwardIssue) {
				return true
			}
		}
	}
	return false
}

// FindBlockers returns the list of unresolved blocking issues.
// A blocker is an issue linked via "is blocked by" whose status category is not "done".
func FindBlockers(issue *api.Issue) []api.LinkedIssue {
	var blockers []api.LinkedIssue
	for _, link := range issue.Fields.IssueLinks {
		if link.Type == nil {
			continue
		}
		// "is blocked by" is the inward description of a Blocks link.
		// When viewing from the blocked issue, InwardIssue is the blocker.
		if link.Type.Inward == "is blocked by" && link.InwardIssue != nil {
			if !isLinkedIssueDone(link.InwardIssue) {
				blockers = append(blockers, *link.InwardIssue)
			}
		}
	}
	return blockers
}

// isLinkedIssueDone checks if a linked issue's status category is "done".
func isLinkedIssueDone(issue *api.LinkedIssue) bool {
	if issue.Fields == nil || issue.Fields.Status == nil || issue.Fields.Status.StatusCategory == nil {
		return false
	}
	return issue.Fields.Status.StatusCategory.Key == CategoryDone
}

// FindTransitionByCategory finds a transition whose target status
// matches the given category key ("indeterminate", "done", "new").
func FindTransitionByCategory(transitions []api.Transition, category string) (*api.Transition, error) {
	for i := range transitions {
		t := &transitions[i]
		if t.To != nil && t.To.StatusCategory != nil && t.To.StatusCategory.Key == category {
			return t, nil
		}
	}

	available := make([]map[string]interface{}, 0, len(transitions))
	for _, t := range transitions {
		entry := map[string]interface{}{
			"id":   t.ID,
			"name": t.Name,
		}
		if t.To != nil {
			entry["toStatus"] = t.To.Name
			if t.To.StatusCategory != nil {
				entry["category"] = t.To.StatusCategory.Key
			}
		}
		available = append(available, entry)
	}

	return nil, clierrors.NewTransitionError(
		"No transition found with target status category "+category,
		available,
	).WithSuggestion("Run 'jira issue transitions <key>' to see available transitions")
}

// MapPriorityRank converts a Jira priority name to a numeric rank (0-4).
// Lower rank = higher priority. Unknown priorities get rank 2 (Medium).
func MapPriorityRank(priority *api.Priority) int {
	if priority == nil {
		return 2
	}
	switch strings.ToLower(priority.Name) {
	case "highest":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	case "lowest":
		return 4
	default:
		return 2
	}
}

// SortByPriorityThenCreated sorts issues by priority rank (ascending),
// then by created date (ascending) for stable ordering.
func SortByPriorityThenCreated(issues []api.Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		ri := MapPriorityRank(issues[i].Fields.Priority)
		rj := MapPriorityRank(issues[j].Fields.Priority)
		if ri != rj {
			return ri < rj
		}
		return issues[i].Fields.Created < issues[j].Fields.Created
	})
}

// ResolveIssueTypeName returns the project's own name for a sub-task or a
// standard issue type.
//
// The names are not fixed across Jira: a company-managed project calls its
// sub-task type "Sub-task" while a team-managed one calls it "Subtask", and a
// project need not have a type called "Task" at all. Hardcoding either name
// makes the command fail on half the projects in existence.
//
// fallback is returned when the project's metadata cannot be read — the caller
// is better off attempting the create and surfacing Jira's own error than
// failing early on a permissions problem.
func ResolveIssueTypeName(ctx context.Context, client *api.Client, project string, wantSubtask bool, fallback string) string {
	meta, err := client.GetCreateMeta(ctx, project)
	if err != nil {
		return fallback
	}
	for _, it := range meta.IssueTypes {
		if it.Subtask == wantSubtask {
			return it.Name
		}
	}
	return fallback
}

// AgentReadyFields returns the Jira fields needed for ready queue computation.
func AgentReadyFields() []string {
	return []string{
		"summary", "status", "priority", "issuetype",
		"labels", "issuelinks", "parent", "created", "updated",
		"assignee", "components",
	}
}

// sprintInfo is the display model for sprint metadata used by status and prime.
type sprintInfo struct {
	Name          string `json:"name"`
	Goal          string `json:"goal,omitempty"`
	EndDate       string `json:"end_date,omitempty"`
	RemainingDays int    `json:"remaining_days"`
}

// toSprintInfo converts an api.Sprint to a sprintInfo display model.
// Returns nil if sprint is nil.
func toSprintInfo(sprint *api.Sprint) *sprintInfo {
	if sprint == nil {
		return nil
	}
	return &sprintInfo{
		Name:          sprint.Name,
		Goal:          sprint.Goal,
		EndDate:       shared.TruncateDate(sprint.EndDate),
		RemainingDays: shared.SprintRemainingDays(sprint.EndDate),
	}
}
