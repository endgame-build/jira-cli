# Sprint-Aware Agent Workflow

## Context

The agent workflow (`agent ready` → `claim` → `close`) currently operates on a flat priority-ordered backlog with blocker detection. It has zero awareness of sprints, boards, or Kanban semantics. For teams using Scrum, the agent can't answer "what's next in this sprint?" — it sees all open issues equally.

**Goal:** Let the agent focus on sprint work when sprints exist, degrade gracefully when they don't.

## Approach: JQL-first + lightweight Agile API metadata

Two orthogonal additions:

1. **Sprint filtering via JQL** — `--sprint` flag on `agent ready` and `issue list` injects `sprint in openSprints()` into the query. Zero new API calls. Works through existing `SearchIssues`.

2. **Sprint metadata via Agile API** — new `DoAgile` method on `Client` for `/rest/agile/1.0/` endpoints. Used by `agent status`, `agent prime`, and new `jira sprint list`/`active` commands to display sprint name, goal, dates, remaining days.

This separation means the critical path (agent picking next task) needs no new HTTP calls, while the metadata path is additive and fails gracefully.

## Changes

### 1. API layer — Agile endpoint support

**`internal/api/client.go`** — Add `DoAgile` method

Extract shared HTTP logic from `Do` into private `doRequest(ctx, method, fullURL, body, out) error`. Both `Do` and `DoAgile` call it with their respective base URLs.

```go
// New field on Client:
agileBaseURL string // "https://{instance}/rest/agile/1.0"

// In NewClient, after baseURL:
agileBaseURL: fmt.Sprintf("https://%s/rest/agile/1.0", creds.Instance),

// New method:
func (c *Client) DoAgile(ctx, method, path, body, out) error {
    return c.doRequest(ctx, method, c.agileBaseURL+"/"+path, body, out)
}

// New test option:
func WithAgileBaseURL(url string) ClientOption
```

**`internal/api/types.go`** — Add Agile types

```go
type Board struct {
    ID       int            `json:"id"`
    Name     string         `json:"name"`
    Type     string         `json:"type"` // "scrum" or "kanban"
    Location *BoardLocation `json:"location,omitempty"`
}

type BoardLocation struct {
    ProjectID   int    `json:"projectId"`
    ProjectKey  string `json:"projectKey"`
    ProjectName string `json:"displayName"`
}

type Sprint struct {
    ID            int    `json:"id"`
    Name          string `json:"name"`
    State         string `json:"state"` // "future", "active", "closed"
    StartDate     string `json:"startDate,omitempty"`
    EndDate       string `json:"endDate,omitempty"`
    CompleteDate  string `json:"completeDate,omitempty"`
    Goal          string `json:"goal,omitempty"`
    OriginBoardID int    `json:"originBoardId"`
}

// Agile API uses offset pagination with `values` wrapper
type BoardPage struct { ... }
type SprintPage struct { ... }
```

**`internal/api/agile.go`** — New file, three methods

```go
func (c *Client) GetBoardsForProject(ctx, projectKey) ([]Board, error)
// GET board?projectKeyOrId={key}&maxResults=50

func (c *Client) GetSprintsForBoard(ctx, boardID int, state string) ([]Sprint, error)
// GET board/{id}/sprint?state={state}&maxResults=50

func (c *Client) GetActiveSprint(ctx, projectKey) (*Sprint, error)
// Convenience: finds first scrum board for project, returns its active sprint.
// Returns nil, nil when: no board, kanban board, no active sprint.
```

### 2. Agent ready — `--sprint` flag

**`internal/cmd/agent/ready.go`**

Add `Sprint string` to `ReadyOptions`. Register flag:
```
--sprint    Filter by sprint (active, future, or sprint name)
```

In `buildReadyJQL`, add clause based on value:
- `"active"` → `sprint in openSprints()`
- `"future"` → `sprint in futureSprints()`
- any other string → `sprint = "<name>"`

No Agile API call. Pure JQL injection.

### 3. Issue list — `--sprint` flag

**`internal/cmd/issue/list.go`**

Add `Sprint string` to `ListOptions`. Register flag same as above.

In `buildJQL`, add same clause logic. When `--jql` is set, `--sprint` is ignored (consistent with other filter flags being overridden).

### 4. Agent status — sprint context

**`internal/cmd/agent/status.go`**

Add optional sprint info to `statusResult`:

```go
type statusResult struct {
    // ... existing fields ...
    Sprint *sprintInfo `json:"sprint,omitempty"`
}

type sprintInfo struct {
    Name          string `json:"name"`
    Goal          string `json:"goal,omitempty"`
    EndDate       string `json:"end_date,omitempty"`
    RemainingDays int    `json:"remaining_days"`
}
```

After existing queries, attempt `client.GetActiveSprint(ctx, project)`. On failure, log warning and continue. On success, compute remaining days from `EndDate` and populate `Sprint` field.

Text output adds a line: `Sprint: Sprint 42 (ends 2026-04-10, 4 days left)`

### 5. Agent prime — sprint context

**`internal/cmd/agent/prime.go`**

After fetching project metadata, attempt `GetActiveSprint`. If found, add section:

```
## Sprint: Sprint 42
- **Goal:** Complete auth refactor
- **Ends:** 2026-04-10 (4 days remaining)
- **Tip:** Use `--sprint active` with `agent ready` to focus on sprint work
```

Update session protocol step 1 to show `--sprint active` option.

### 6. Sprint commands — new command group

**`internal/cmd/sprint/`** — new directory

**`sprint.go`** — group registration:
```go
func NewCmdSprint(f *factory.Factory) *cobra.Command
// Subcommands: list, active
```

**`list.go`** — `jira sprint list`:
```
--project, -p   Project key (falls back to default.project)
--state         Filter by state: active, future, closed (default: all)
--board         Board ID (auto-detected from project if omitted)
```
Output table: NAME | STATE | START | END | GOAL

**`active.go`** — `jira sprint active`:
```
--project, -p   Project key (falls back to default.project)
```
Shows active sprint detail: name, goal, dates, remaining days, board name.

**`internal/cmd/root/root.go`** — register:
```go
cmd.AddCommand(sprint.NewCmdSprint(f))
```

### 7. Tests

| File | What |
|------|------|
| `internal/api/agile_test.go` | `httptest.Server` mocking `/rest/agile/1.0/board`, `/sprint`. Test board lookup, sprint filtering, no-board case. |
| `internal/cmd/agent/ready_test.go` | Add cases: `--sprint active` produces JQL with `sprint in openSprints()`, `--sprint "Sprint 1"` produces `sprint = "Sprint 1"` |
| `internal/cmd/issue/list_test.go` | Same JQL verification for `--sprint` flag |
| `internal/cmd/sprint/list_test.go` | Table output, `--state active` filtering, JSON output |
| `internal/cmd/sprint/active_test.go` | Active sprint display, no-active-sprint case, kanban case |

## Edge Cases

| Scenario | Behavior |
|----------|----------|
| No board for project | `GetActiveSprint` returns `nil, nil`. Status/prime omit sprint section. `--sprint active` on ready still works via JQL. |
| Kanban board | Same — JQL `sprint in openSprints()` returns empty. No error, just no results. |
| Multiple boards | Use first scrum board. If only kanban boards, return nil. |
| No active sprint (between sprints) | `GetActiveSprint` returns nil. Status shows no sprint. `--sprint active` returns empty ready queue. |
| Agile API permissions denied | `GetActiveSprint` returns error. Logged as warning. JQL sprint functions still work independently. |

## Implementation Sequence

1. **API layer**: `client.go` (DoAgile + refactor Do), `types.go` (agile types), `agile.go`, `agile_test.go`
2. **JQL sprint filter**: `ready.go` + `list.go` (--sprint flag), tests
3. **Sprint metadata**: `status.go` + `prime.go` (sprint info), tests
4. **Sprint commands**: `sprint/` directory, `root.go` registration, tests

Steps 1-2 deliver the core value. Steps 3-4 add polish.

## Verification

```sh
# After step 1 — API layer
go test ./internal/api/...

# After step 2 — Sprint filtering
go test ./internal/cmd/agent/... ./internal/cmd/issue/...
# Manual: jira agent ready --sprint active --project PROJ

# After step 3 — Sprint metadata
go test ./internal/cmd/agent/...
# Manual: jira agent status --project PROJ  (should show sprint line)
# Manual: jira agent prime --project PROJ   (should show sprint section)

# After step 4 — Sprint commands
go test ./internal/cmd/sprint/...
# Manual: jira sprint list --project PROJ
# Manual: jira sprint active --project PROJ

# Full suite
make test && make lint
```

## Files Summary

| Action | File |
|--------|------|
| Modify | `internal/api/client.go` — add `agileBaseURL`, `DoAgile`, refactor `Do` |
| Modify | `internal/api/types.go` — add Board, Sprint, page types |
| Create | `internal/api/agile.go` — agile API methods |
| Create | `internal/api/agile_test.go` |
| Modify | `internal/cmd/agent/ready.go` — add `--sprint` flag + JQL clause |
| Modify | `internal/cmd/agent/status.go` — add sprint info |
| Modify | `internal/cmd/agent/prime.go` — add sprint context |
| Modify | `internal/cmd/issue/list.go` — add `--sprint` flag + JQL clause |
| Create | `internal/cmd/sprint/sprint.go` — command group |
| Create | `internal/cmd/sprint/list.go` |
| Create | `internal/cmd/sprint/active.go` |
| Create | `internal/cmd/sprint/list_test.go` |
| Create | `internal/cmd/sprint/active_test.go` |
| Modify | `internal/cmd/root/root.go` — register sprint command |
