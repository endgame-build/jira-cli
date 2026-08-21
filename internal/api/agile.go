package api

import (
	"context"
	"fmt"
	"net/http"
)

const agilePageSize = 50

// fetchAllAgile exhausts offset-based pagination for agile endpoints.
// The fetcher is called with increasing startAt until isLast is true
// or an empty page is returned.
func fetchAllAgile[T any](
	ctx context.Context,
	fetcher func(startAt int) (values []T, isLast bool, err error),
) ([]T, error) {
	var all []T
	startAt := 0
	for {
		values, isLast, err := fetcher(startAt)
		if err != nil {
			return nil, err
		}
		all = append(all, values...)
		if isLast || len(values) == 0 {
			break
		}
		startAt += len(values)
	}
	return all, nil
}

// GetBoardsForProject returns all boards associated with a project key.
// Returns an empty slice (not an error) when the project has no boards.
func (c *Client) GetBoardsForProject(ctx context.Context, projectKey string) ([]Board, error) {
	return fetchAllAgile(ctx, func(startAt int) ([]Board, bool, error) {
		path := fmt.Sprintf("board?projectKeyOrId=%s&maxResults=%d&startAt=%d",
			projectKey, agilePageSize, startAt)

		var page boardPage
		if err := c.DoAgile(ctx, http.MethodGet, path, nil, &page); err != nil {
			return nil, false, err
		}
		return page.Values, page.IsLast, nil
	})
}

// GetSprintsForBoard returns sprints for a board, optionally filtered by state.
// state: "active", "future", "closed", or "" for all.
func (c *Client) GetSprintsForBoard(ctx context.Context, boardID int, state string) ([]Sprint, error) {
	return fetchAllAgile(ctx, func(startAt int) ([]Sprint, bool, error) {
		path := fmt.Sprintf("board/%d/sprint?maxResults=%d&startAt=%d",
			boardID, agilePageSize, startAt)
		if state != "" {
			path += "&state=" + state
		}

		var page sprintPage
		if err := c.DoAgile(ctx, http.MethodGet, path, nil, &page); err != nil {
			return nil, false, err
		}
		return page.Values, page.IsLast, nil
	})
}

// GetActiveSprint returns the active sprint for the first sprint-capable board
// in a project.
// Returns nil, nil when: no sprint-capable boards or no active sprint.
// Returns nil, err only on API/network failure.
func (c *Client) GetActiveSprint(ctx context.Context, projectKey string) (*Sprint, error) {
	// Fetch only sprint-capable boards — avoids paginating through kanban boards.
	boards, err := c.getSprintBoards(ctx, projectKey)
	if err != nil {
		return nil, err
	}
	if len(boards) == 0 {
		return nil, nil
	}

	sprints, err := c.GetSprintsForBoard(ctx, boards[0].ID, "active")
	if err != nil {
		return nil, err
	}
	if len(sprints) == 0 {
		return nil, nil
	}

	return &sprints[0], nil
}

// SprintBoardTypes lists the board types that can carry sprints.
//
// A company-managed Scrum board reports "scrum"; a team-managed one reports
// "simple" while carrying sprints exactly the same way. Filtering on "scrum"
// alone makes every sprint feature invisible on team-managed projects, which
// are the default project type in Jira Cloud.
const SprintBoardTypes = "scrum,simple"

// IsSprintBoardType reports whether a board type can carry sprints.
func IsSprintBoardType(boardType string) bool {
	return boardType == "scrum" || boardType == "simple"
}

// getSprintBoards returns the sprint-capable boards for a project, filtered
// server-side. The Agile API accepts a comma-separated type list.
func (c *Client) getSprintBoards(ctx context.Context, projectKey string) ([]Board, error) {
	return fetchAllAgile(ctx, func(startAt int) ([]Board, bool, error) {
		path := fmt.Sprintf("board?projectKeyOrId=%s&type=%s&maxResults=%d&startAt=%d",
			projectKey, SprintBoardTypes, agilePageSize, startAt)

		var page boardPage
		if err := c.DoAgile(ctx, http.MethodGet, path, nil, &page); err != nil {
			return nil, false, err
		}
		return page.Values, page.IsLast, nil
	})
}

// AddIssuesToSprint moves issues into a sprint.
//
// POST /rest/agile/1.0/sprint/{sprintId}/issue, which returns 204 with no body.
// Jira caps this at 50 issues per call.
func (c *Client) AddIssuesToSprint(ctx context.Context, sprintID int, issueKeys []string) error {
	path := fmt.Sprintf("sprint/%d/issue", sprintID)
	body := map[string]interface{}{"issues": issueKeys}
	return c.DoAgile(ctx, http.MethodPost, path, body, nil)
}
