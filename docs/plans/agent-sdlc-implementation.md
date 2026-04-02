# Implementation Plan: `jira agent` Command Group

## Overview

Add a `jira agent` command group that provides an agentic SDLC backed by Jira Cloud. Agents execute a `ready → claim → work → discover → close` loop using Jira issues, links, and transitions as the backing store.

## Existing Building Blocks

All API operations needed already exist in jira-cli:

| Need | Existing Component | Location |
|------|-------------------|----------|
| Search issues | `api.SearchIssues()` | `internal/api/issues.go` |
| Get transitions | `api.GetTransitions()` | `internal/api/issues.go` |
| Do transition | `api.DoTransition()` | `internal/api/issues.go` |
| Assign issue | `api.AssignIssue()` | `internal/api/issues.go` |
| Create issue | `api.CreateIssue()` | `internal/api/issues.go` |
| Get issue | `api.GetIssue()` | `internal/api/issues.go` |
| Add comment | `api.AddComment()` | `internal/api/comments.go` |
| Resolve user (@me) | `shared.ResolveUserAccountID()` | `internal/cmd/shared/` |
| ADF conversion | `adf.MarkdownToADF()` | `internal/adf/` |
| JSON/text output | `output.Formatter` | `internal/output/` |
| Structured errors | `CLIError` | `internal/errors/` |
| Batch pattern | reconcile command | `internal/cmd/issue/reconcile.go` |
| JQL builder | list command | `internal/cmd/issue/list.go` |

## New Files

```
internal/cmd/agent/
├── agent.go          # Root command, registers subcommands
├── ready.go          # Ready queue (search + blocker filter)
├── claim.go          # Assign + transition to In Progress
├── close.go          # Transition to Done + comment
├── discover.go       # Create linked/sub-task issue
├── prime.go          # Context injection output
├── status.go         # Current work summary
├── blocked.go        # Blocked issues with blocker details
├── shared.go         # Blocker check, status category matching, priority mapping
├── ready_test.go
├── claim_test.go
├── close_test.go
├── discover_test.go
├── prime_test.go
├── status_test.go
├── blocked_test.go
└── shared_test.go
```

## Modified Files

```
internal/cmd/root/root.go    # Add: agent.NewCmdAgent(f)
internal/api/issues.go       # May need: expand=issuelinks in search options
```

## Implementation Sequence

### Step 1: Shared Helpers (`shared.go`)

Foundation that all commands depend on.

```go
// IsBlocked checks if an issue has unresolved "is blocked by" links.
// Examines issuelinks field from Jira response.
func IsBlocked(issue *api.Issue) bool

// FindBlockers returns the list of unresolved blocking issues.
func FindBlockers(issue *api.Issue) []api.Issue

// FindTransitionByCategory finds a transition whose target status
// matches the given category key ("indeterminate", "done", "new").
func FindTransitionByCategory(transitions []api.Transition, category string) (*api.Transition, error)

// MapPriorityRank converts Jira priority name to numeric rank (0-4).
func MapPriorityRank(priority *api.Priority) int

// AgentReadyFields returns the Jira fields needed for ready queue computation.
func AgentReadyFields() []string
// Returns: summary, status, priority, issuetype, labels, issuelinks, parent, created, updated
```

### Step 2: Ready Queue (`ready.go`)

```go
type ReadyOptions struct {
    Factory   *factory.Factory
    Project   string
    Limit     int
    Assignee  string
    Unassigned bool
    Type      string
    Labels    []string
    Priority  string
    Sort      string
}
```

Algorithm:
1. Build JQL: `project = X AND statusCategory != done AND statusCategory != indeterminate`
2. Add filters for assignee, type, labels, priority
3. Search with `fields=summary,status,priority,issuetype,labels,issuelinks,parent,created,updated`
4. Post-filter: exclude issues where `IsBlocked()` returns true
5. Sort by priority rank, then created date
6. Truncate to `--limit`
7. Output via `formatter.OutputList()`

### Step 3: Claim (`claim.go`)

```go
type ClaimOptions struct {
    Factory *factory.Factory
    KeyOrID string
    Force   bool
}
```

Algorithm:
1. Get issue (validate key, check current state)
2. If already claimed by me and in-progress → return idempotent success
3. If claimed by another and no --force → return CONFLICT_ERROR
4. Assign to @me: `api.AssignIssue()`
5. Get transitions, find "In Progress" via `FindTransitionByCategory("indeterminate")`
6. Do transition: `api.DoTransition()`
7. On transition failure → rollback assignment (set assignee to previous)
8. Output via `formatter.OutputMutation()`

### Step 4: Close (`close.go`)

```go
type CloseOptions struct {
    Factory     *factory.Factory
    KeyOrID     string
    Reason      string
    SuggestNext bool
    ClaimNext   bool
}
```

Algorithm:
1. Get issue, check not already done (idempotent if so)
2. Get transitions, find "Done" via `FindTransitionByCategory("done")`
3. Do transition
4. If reason provided → add comment (markdown → ADF)
5. If suggest-next or claim-next:
   a. Find issues that have "is blocked by" link to this key
   b. For each: re-check `IsBlocked()` (all blockers resolved now?)
   c. Return newly unblocked
6. If claim-next → auto-claim top unblocked issue
7. Output via `formatter.OutputMutation()` with `unblocked` field

### Step 5: Discover (`discover.go`)

```go
type DiscoverOptions struct {
    Factory     *factory.Factory
    ParentKey   string
    Title       string
    Description string
    Type        string
    Priority    string
    Labels      []string
    AsSubtask   bool
    LinkType    string
    BodyFile    string
}
```

Algorithm:
1. Fetch parent issue (inherit project, priority, labels)
2. Determine if sub-task or linked issue
3. Build CreateIssueInput with inherited + overridden fields
4. If sub-task: set `parent: {key: parentKey}` in fields
5. Create issue: `api.CreateIssue()`
6. If not sub-task: create issue link (type: Relates or --link-type)
7. Add "discovered" label
8. Add comment to parent: "Discovered PROJ-456: <title>"
9. Output via `formatter.OutputMutation()`

### Step 6: Prime (`prime.go`)

```go
type PrimeOptions struct {
    Factory *factory.Factory
    Project string
    Full    bool
}
```

Algorithm:
1. Resolve project (flag → config → error)
2. Fetch project metadata: `api.GetCreateMeta()` for issue types
3. Build markdown template with:
   - Rules section (use jira agent, no TodoWrite, etc.)
   - Core commands reference
   - Session protocol
   - Project-specific types and priorities
4. Output to stdout (raw markdown, not JSON)

### Step 7: Status + Blocked (`status.go`, `blocked.go`)

Status: Three JQL queries (ready count, in-progress, done today) + format.
Blocked: Same as ready but return only `IsBlocked() == true` with blocker details.

### Step 8: Register in Root (`root.go`)

Add to command registration:

```go
import "github.com/endgame-build/jira-cli/internal/cmd/agent"

// In NewCmdRoot():
rootCmd.AddCommand(agent.NewCmdAgent(f))
```

## API Changes

### `SearchOptions` Enhancement

May need to ensure `issuelinks` is included in search fields. Check if `SearchIssues` already supports arbitrary field lists.

Current `SearchOptions`:
```go
type SearchOptions struct {
    JQL           string
    Fields        []string  // ← already supports custom fields
    MaxResults    int
    NextPageToken string
}
```

This already works — just pass `issuelinks` in the Fields slice.

### Issue Link Type Access

Need to read `issue.Fields.IssueLinks` — this field already exists in the `IssueFields` struct as `[]IssueLink`. Verify the `IssueLink` struct has enough detail (linked issue status, link type direction).

Current `IssueLink` struct:
```go
type IssueLink struct {
    ID           string    `json:"id"`
    Type         LinkType  `json:"type"`
    InwardIssue  *Issue    `json:"inwardIssue"`
    OutwardIssue *Issue    `json:"outwardIssue"`
}
```

This has what we need: link type (with inward/outward names) and linked issue (with status).

## Design Decisions

### D1: Status matching by category, not name

Jira projects have custom workflow statuses ("Open", "In Review", "QA", etc.). Matching by `statusCategory.key` (`new`, `indeterminate`, `done`) is portable.

### D2: Dependency detection in one API call

Include `issuelinks` in search fields. Post-filter in code. Avoids N+1 API calls.

### D3: Sub-task vs linked issue for discoveries

Default to sub-task when parent type supports it (most Jira projects do). Fall back to linked issue when parent is already a sub-task (sub-sub-tasks not supported in Jira).

### D4: Close reason as comment

Jira has no native "close reason" field. Adding a comment with "Closed: <reason>" prefix is a convention that works universally without custom field configuration.

### D5: No local state storage (Phase 1)

Phase 1 relies entirely on Jira as the backing store. No local files for agent state, memories, or session tracking. This keeps the implementation simple and stateless. Local persistence (memories, session context) is Phase 3 scope.

## Test Strategy

Each command gets table-driven tests using existing patterns:

```go
func TestReady(t *testing.T) {
    tests := []struct {
        name       string
        serverResp string   // mock Jira response
        wantKeys   []string // expected ready issue keys
        wantErr    bool
    }{
        {
            name: "filters out blocked issues",
            // ... mock response with blocked + unblocked issues
            wantKeys: []string{"PROJ-1", "PROJ-3"}, // only unblocked
        },
        {
            name: "empty when all blocked",
            // ... all issues have unresolved blockers
            wantKeys: []string{},
        },
    }
}
```

**Test setup:**
- `factory.NewTestFactory(ios, cfg, client)` — pre-wired, no credentials
- `httptest.NewServer()` — mock Jira API responses
- `iostreams.Test()` — capture stdout/stderr

**Key test scenarios:**
- Ready: blocked issues excluded, priority ordering, empty queue
- Claim: idempotent, conflict detection, no valid transition
- Close: already closed (idempotent), suggest-next unblocked detection
- Discover: sub-task creation, linked issue fallback, label inheritance
- Prime: project-specific output, full mode

## Verification

```bash
# Build
make build

# Unit tests
go test ./internal/cmd/agent/...

# Integration (requires Jira instance)
bin/jira agent ready --project PROJ --json
bin/jira agent claim PROJ-123
bin/jira agent close PROJ-123 --reason "implemented"
bin/jira agent discover PROJ-100 --title "Found edge case"
bin/jira agent prime --project PROJ

# Lint
make lint
```

## Phases

| Phase | Scope | Commands |
|-------|-------|----------|
| 1 | Core loop | ready, claim, close, discover, prime, shared |
| 2 | Visibility | status, blocked |
| 3 | Session integration | Claude Code hooks, local memories, AGENTS.md template |
