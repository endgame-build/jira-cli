//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/endgame-build/jira-cli/internal/api"
	"github.com/endgame-build/jira-cli/internal/auth"
)

// Sandbox holds the validated facts about the test project. Tests read these
// rather than hardcoding names, because they differ between company-managed and
// team-managed Jira projects.
type Sandbox struct {
	Client     *api.Client
	ProjectKey string
	AccountID  string
	Board      api.Board
	Sprint     api.Sprint

	// SubtaskType is the project's actual sub-task type name. Team-managed
	// projects call it "Subtask"; company-managed ones call it "Sub-task".
	// internal/cmd/agent/discover.go:107 hardcodes "Sub-task", so on a
	// team-managed project the default `agent discover` invocation fails.
	// Chain tests pass --type explicitly; TestE2E_DISCOVER_04 pins the bug.
	SubtaskType string

	// TaskType is the project's plain task type name.
	TaskType string

	// CLISeesBoard records whether the CLI's own board lookup can find this
	// board. It is answered by running `sprint active` rather than by
	// inspecting the board type, so it stays accurate as the lookup changes.
	// TestE2E_SPRINT_06 pins the defect that makes it false; the cases that
	// depend on that path skip when it is.
	CLISeesBoard bool
}

var (
	preflightOnce sync.Once
	sandbox       *Sandbox
	preflightErr  error
)

const setupHint = `Set up the sandbox:
  1. Create a Scrum project whose key is LETTERS ONLY (e.g. AGENTLAB).
     A key containing a digit makes every claim/close/discover call fail with
     exit 3 before any HTTP request — see internal/cmd/shared/validate.go:12.
  2. Create a sprint on its board and START it, with an end date at least a day out.
  3. Ensure the project has a Task type, a sub-task type, and priorities Highest..Lowest.
  4. export JIRA_INSTANCE / JIRA_USER / JIRA_TOKEN / JIRA_E2E_PROJECT
See test/e2e/README.md.`

// preflight validates the sandbox once per process. A misconfigured sandbox is
// a hard failure rather than a skip: a skipped e2e suite and a passing one look
// identical in CI output, which is how regressions get shipped.
func preflight(t *testing.T) *Sandbox {
	t.Helper()
	preflightOnce.Do(func() { sandbox, preflightErr = checkSandbox() })
	if preflightErr != nil {
		t.Fatalf("e2e sandbox is not usable:\n\n  %v\n\n%s", preflightErr, setupHint)
	}
	return sandbox
}

// bootstrapClient builds an API client from the env credentials. The suite
// requires the env triple so the child process and the fixture layer are
// guaranteed to talk to the same instance as the same user.
func bootstrapClient() (*api.Client, error) {
	instance, user, token := os.Getenv(EnvInstance), os.Getenv(EnvUser), os.Getenv(EnvToken)
	if instance == "" || user == "" || token == "" {
		return nil, fmt.Errorf("%s, %s and %s must all be set", EnvInstance, EnvUser, EnvToken)
	}
	return api.NewClient(&auth.Credentials{
		Instance: auth.NormalizeInstanceURL(instance),
		User:     user,
		Token:    token,
	}), nil
}

func checkSandbox() (*Sandbox, error) {
	ctx := context.Background()

	projectKey := os.Getenv(EnvProject)
	if projectKey == "" {
		return nil, fmt.Errorf("%s is not set", EnvProject)
	}

	client, err := bootstrapClient()
	if err != nil {
		return nil, err
	}

	me, err := client.GetMyself(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot authenticate against %s as %s: %w",
			os.Getenv(EnvInstance), os.Getenv(EnvUser), err)
	}

	if _, err := client.GetProject(ctx, projectKey); err != nil {
		return nil, fmt.Errorf("project %q is not visible to %s: %w", projectKey, me.DisplayName, err)
	}

	boards, err := client.GetBoardsForProject(ctx, projectKey)
	if err != nil {
		return nil, fmt.Errorf("cannot list boards for %q: %w", projectKey, err)
	}

	// Accept "simple" as well as "scrum". A team-managed project's board carries
	// sprints but reports its type as "simple", and preflight has to describe the
	// instance as it really is rather than as the CLI wishes it were — otherwise
	// the suite would refuse to run on exactly the projects where the sprint
	// commands are broken, and the defect would stay invisible.
	var board *api.Board
	for i := range boards {
		if boards[i].Type == "scrum" || boards[i].Type == "simple" {
			board = &boards[i]
			break
		}
	}
	if board == nil {
		return nil, fmt.Errorf("project %q has no board carrying sprints; the sprint commands need one", projectKey)
	}

	sprints, err := client.GetSprintsForBoard(ctx, board.ID, "active")
	if err != nil {
		return nil, fmt.Errorf("cannot list active sprints on board %d: %w", board.ID, err)
	}
	if len(sprints) != 1 {
		return nil, fmt.Errorf("board %d has %d active sprints, want exactly 1 — start a sprint, or close the extras",
			board.ID, len(sprints))
	}
	sprint := sprints[0]
	if sprint.EndDate == "" {
		return nil, fmt.Errorf("active sprint %q has no end date; the remaining_days assertions need one", sprint.Name)
	}

	meta, err := client.GetCreateMeta(ctx, projectKey)
	if err != nil {
		return nil, fmt.Errorf("cannot read create metadata for %q: %w", projectKey, err)
	}
	var taskType, subtaskType string
	for _, it := range meta.IssueTypes {
		switch {
		case it.Subtask && subtaskType == "":
			subtaskType = it.Name
		case !it.Subtask && it.Name == "Task":
			taskType = it.Name
		}
	}
	if taskType == "" {
		return nil, fmt.Errorf("project %q has no %q issue type", projectKey, "Task")
	}
	if subtaskType == "" {
		return nil, fmt.Errorf("project %q has no sub-task issue type", projectKey)
	}

	return &Sandbox{
		Client:      client,
		ProjectKey:  projectKey,
		AccountID:   me.AccountID,
		Board:       *board,
		Sprint:      sprint,
		SubtaskType: subtaskType,
		TaskType:    taskType,
	}, nil
}
