# Agentic SDLC Contracts

Reference specification for agentic development lifecycle commands in jira-cli, derived from beads (`bd`) CLI contracts. These contracts define what goes in, what comes out, and what invariants must hold.

## Core Loop

```
ready → claim → work → discover → close → repeat
```

An agent executes this loop continuously. Each step has a defined contract below.

---

## Contract 1: Ready Queue

**Purpose:** Return only issues that an agent can start working on right now — no unresolved blockers, not already claimed, not deferred, not closed.

### Input

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `--project` | string | config default | Jira project key |
| `--limit` | int | 10 | Max issues to return |
| `--assignee` | string | (none) | Filter by assignee |
| `--unassigned` | bool | false | Only unassigned issues |
| `--type` | string | (none) | Filter by issue type (Task, Bug, Story, Epic) |
| `--label` | string[] | (none) | Filter by labels (AND) |
| `--priority` | string | (none) | Filter by priority (Highest, High, Medium, Low, Lowest) |
| `--component` | string | (none) | Filter by component |
| `--sort` | string | priority | Sort: priority, created, updated |
| `--json` | bool | false | JSON output |
| `--jq` | string | (none) | jq filter expression |

### Output

JSON array of issue objects. Empty `[]` when no work available.

```json
{
  "data": [
    {
      "key": "PROJ-123",
      "summary": "Implement login validation",
      "status": { "name": "To Do", "category": "new" },
      "priority": { "name": "High", "rank": 1 },
      "type": "Task",
      "labels": ["backend"],
      "created": "2026-03-15T10:00:00.000+0000",
      "updated": "2026-03-20T14:30:00.000+0000",
      "parent": "PROJ-100",
      "blockers": [],
      "blocker_count": 0
    }
  ],
  "pagination": {
    "total": 5,
    "limit": 10
  }
}
```

### Algorithm

```
1. JQL search: project = PROJ AND statusCategory != done
     AND (assignee IS EMPTY OR assignee = <filter>)
     AND type IN (<filter>) AND labels IN (<filter>)
     ORDER BY priority ASC, created ASC
   Fields: summary, status, priority, issuetype, labels, issuelinks, parent, created, updated

2. For each issue in results:
   a. Extract inward issuelinks where type.name = "Blocks" (i.e., "is blocked by" links)
   b. For each blocker link:
      - If blocker issue's status category != "done" → issue is BLOCKED
   c. If no unresolved blockers → issue is READY

3. Return only READY issues, up to --limit
```

### Invariants

1. **Never returns blocked issues.** If issue X "is blocked by" issue Y, and Y is not Done → X is excluded.
2. **Never returns in-progress issues.** Status category `indeterminate` (In Progress) is excluded.
3. **Never returns done/closed issues.** Status category `done` is excluded.
4. **Priority ordering is stable.** Highest (rank 0) before High (rank 1) before Medium (rank 2).
5. **Empty result is valid.** `[]` means no actionable work — agent should stop or request more work.

### Failure Modes

| Failure | Behavior |
|---------|----------|
| Auth failure | CLIError code=AUTH_ERROR, exit 2 |
| Project not found | CLIError code=NOT_FOUND, exit 4 |
| Network error | CLIError code=NETWORK_ERROR, exit 7, suggestion to retry |
| No results | Success with empty `data: []` (not an error) |

---

## Contract 2: Claim

**Purpose:** Atomically assign an issue to the caller and transition it to "In Progress". Idempotent if already claimed by the same user.

### Input

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `key` | string | yes | Jira issue key (positional arg) |
| `--force` | bool | no | Claim even if assigned to someone else |
| `--json` | bool | no | JSON output |

### Output

```json
{
  "ok": true,
  "key": "PROJ-123",
  "assignee": "user@example.com",
  "status": "In Progress",
  "previous_status": "To Do",
  "previous_assignee": null
}
```

### Algorithm

```
1. Fetch issue: GET /rest/api/3/issue/{key}?fields=status,assignee
2. Check current state:
   a. If assignee == caller AND status.category == "indeterminate" → idempotent success (no-op)
   b. If assignee != null AND assignee != caller AND !--force → error CONFLICT
3. Assign: PUT /rest/api/3/issue/{key}/assignee  (body: {"accountId": "<me>"})
4. Get transitions: GET /rest/api/3/issue/{key}/transitions
5. Find transition where target status category == "indeterminate" (In Progress)
6. Transition: POST /rest/api/3/issue/{key}/transitions  (body: {"transition": {"id": "..."}})
7. If transition fails → rollback assignment (best effort)
```

### Invariants

1. **Idempotent for same caller.** Claiming your own in-progress issue is a no-op success.
2. **Rejects conflict by default.** If assigned to someone else, returns CONFLICT_ERROR unless `--force`.
3. **Sequential but safe.** Jira doesn't support true atomic assign+transition, so we do assign→transition with rollback on failure.
4. **Finds "In Progress" by category, not name.** Status category `indeterminate` matches regardless of project workflow configuration.

### Failure Modes

| Failure | Behavior |
|---------|----------|
| Issue not found | CLIError code=NOT_FOUND |
| Already claimed by another (no --force) | CLIError code=CONFLICT_ERROR, suggestion: use --force |
| No valid transition to In Progress | CLIError code=INVALID_TRANSITION, suggestion: check workflow |
| Issue already Done | CLIError code=VALIDATION_ERROR, message: cannot claim closed issue |
| Transition fails after assignment | Rollback assignment, report original error |

---

## Contract 3: Close

**Purpose:** Transition issue to Done and optionally record a close reason as a comment.

### Input

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `key` | string | yes | Jira issue key (positional arg) |
| `--reason` | string | no | Close reason (added as comment) |
| `--suggest-next` | bool | no | After closing, run ready queue and show newly unblocked work |
| `--claim-next` | bool | no | After closing, auto-claim the top ready issue |
| `--json` | bool | no | JSON output |

### Output

```json
{
  "ok": true,
  "key": "PROJ-123",
  "status": "Done",
  "previous_status": "In Progress",
  "reason": "Implemented with tests passing",
  "unblocked": ["PROJ-124", "PROJ-125"]
}
```

The `unblocked` field lists issues that were blocked by this issue and are now ready (all their blockers resolved). Populated when `--suggest-next` or `--claim-next`.

### Algorithm

```
1. Get transitions: GET /rest/api/3/issue/{key}/transitions
2. Find transition where target status category == "done"
3. Transition: POST /rest/api/3/issue/{key}/transitions
4. If --reason provided:
   POST /rest/api/3/issue/{key}/comment  (body: ADF-formatted reason)
5. If --suggest-next or --claim-next:
   a. Find issues that link "is blocked by" this key
   b. For each: re-check if ALL their blockers are now resolved
   c. Return newly unblocked issues
6. If --claim-next:
   Auto-claim the highest priority unblocked issue (Contract 2)
```

### Invariants

1. **Closing already-closed issue is idempotent.** If status category is already `done`, return success.
2. **Close reason is a comment, not a field.** Convention: comment starts with "Closed: " prefix for agent-generated close reasons.
3. **Unblocked detection is accurate.** Only reports issues where ALL blockers (not just this one) are resolved.

### Failure Modes

| Failure | Behavior |
|---------|----------|
| No transition to Done available | CLIError code=INVALID_TRANSITION |
| Issue not found | CLIError code=NOT_FOUND |
| Comment creation fails | Warning (close still succeeds), error in stderr |

---

## Contract 4: Discover

**Purpose:** Create a new issue linked to the current work item as a discovery. Used when an agent finds bugs, tech debt, or additional work during implementation.

### Input

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `parent-key` | string | yes | The issue being worked on (positional arg) |
| `--title` | string | yes | Summary of discovered issue |
| `--description` | string | no | Full description (markdown, converted to ADF) |
| `--type` | string | no | Issue type (default: Sub-task if parent supports it, else Task) |
| `--priority` | string | no | Priority (default: inherit from parent) |
| `--labels` | string[] | no | Labels (default: inherit from parent + "discovered") |
| `--as-subtask` | bool | no | Create as sub-task (default: true if parent supports sub-tasks) |
| `--link-type` | string | no | Link type if not sub-task (default: "Relates") |
| `--body-file` | string | no | Read description from file |
| `--json` | bool | no | JSON output |
| `--dry-run` | bool | no | Preview without creating |

### Output

```json
{
  "ok": true,
  "key": "PROJ-456",
  "parent": "PROJ-123",
  "relationship": "subtask",
  "summary": "Found: login fails with Unicode usernames",
  "type": "Bug",
  "priority": "High"
}
```

### Algorithm

```
1. Fetch parent issue: GET /rest/api/3/issue/{parent-key}
2. Inherit defaults from parent: project, priority, relevant labels
3. Determine relationship:
   a. If --as-subtask AND parent's issue type supports sub-tasks → create as sub-task (set parent field)
   b. Else → create as standalone issue + add issue link
4. Create issue: POST /rest/api/3/issue  (body: fields with parent or link)
5. If standalone (not sub-task): POST /rest/api/3/issueLink
   (type: "Relates" or --link-type, inwardIssue: new, outwardIssue: parent)
6. Add "discovered" label to new issue
7. Optionally add comment to parent: "Discovered PROJ-456: <title>"
```

### Invariants

1. **Always linked to parent.** Either via sub-task relationship or issue link. No orphan discoveries.
2. **Inherits project from parent.** Cannot create discovery in a different project.
3. **Adds "discovered" label.** Convention for tracking agent-discovered work.
4. **Adds discovery comment to parent.** Breadcrumb trail on the source issue.

---

## Contract 5: Prime (Context Injection)

**Purpose:** Output a self-contained markdown block that gives an agent everything it needs to execute the agentic SDLC loop. Designed for Claude Code SessionStart and PreCompact hooks.

### Input

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `--project` | string | no | Jira project key (default: config) |
| `--full` | bool | no | Include extended command reference |

### Output

Markdown to stdout (NOT JSON). ~1-2k tokens. Contains:

```markdown
# Jira Agent Workflow Context

## Rules
- Use `jira agent` for ALL task tracking
- Do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Create issues BEFORE writing code
- Claim work before starting implementation

## Core Commands
- `jira agent ready` — Find unblocked work
- `jira agent claim <key>` — Assign + move to In Progress
- `jira agent close <key> --reason="..."` — Complete work
- `jira agent discover <parent-key> --title="..."` — File discovered work

## Session Protocol
1. `jira agent ready --json` — find work
2. `jira agent claim <key>` — claim it
3. [implement]
4. `jira agent close <key> --reason="..."` — close
5. `git add && git commit && git push` — push code

## Project: <PROJECT_KEY>
- Statuses: <list of project statuses>
- Types: <list of issue types>
- Priority levels: Highest, High, Medium, Low, Lowest
```

### Invariants

1. **Self-contained.** An agent receiving only this output can execute the full loop.
2. **Project-aware.** Includes actual project statuses and types, not generic ones.
3. **Idempotent.** Running prime multiple times produces the same output.

---

## Contract 6: Status

**Purpose:** Show current agent work status — what's claimed, what's ready, what's blocked.

### Input

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `--project` | string | no | Jira project key |
| `--json` | bool | no | JSON output |

### Output

```json
{
  "project": "PROJ",
  "ready_count": 5,
  "in_progress_count": 2,
  "blocked_count": 3,
  "done_today": 4,
  "my_work": [
    {
      "key": "PROJ-123",
      "summary": "Implement login",
      "status": "In Progress",
      "priority": "High"
    }
  ]
}
```

---

## Contract 7: Blocked

**Purpose:** Show issues that cannot proceed due to unresolved blockers.

### Input

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `--project` | string | no | Jira project key |
| `--limit` | int | 50 | Max issues |
| `--json` | bool | no | JSON output |

### Output

```json
{
  "data": [
    {
      "key": "PROJ-125",
      "summary": "Deploy to staging",
      "status": "To Do",
      "blocked_by": [
        { "key": "PROJ-123", "summary": "Implement login", "status": "In Progress" },
        { "key": "PROJ-124", "summary": "Write tests", "status": "To Do" }
      ]
    }
  ]
}
```

---

## Data Model: Jira ↔ Agent Mapping

### Status Categories

Agent commands match on **status category** (not name) for portability across Jira projects.

| Jira Category | Category Key | Agent Meaning |
|--------------|-------------|--------------|
| To Do | `new` | Open / ready candidate |
| In Progress | `indeterminate` | Claimed / working |
| Done | `done` | Closed / complete |

### Priority Mapping

| Jira Priority | Agent Rank | Description |
|--------------|-----------|-------------|
| Highest | 0 | Critical — fix immediately |
| High | 1 | Important — fix soon |
| Medium | 2 | Normal priority |
| Low | 3 | Can wait |
| Lowest | 4 | Backlog |

### Issue Link Types

| Jira Link Type | Direction | Agent Meaning |
|---------------|-----------|---------------|
| Blocks | outward: "blocks", inward: "is blocked by" | Blocking dependency (affects ready queue) |
| Relates | bidirectional: "relates to" | Informational link |
| Duplicate | outward: "duplicates", inward: "is duplicated by" | Dedup marker |
| Cloners | outward: "clones", inward: "is cloned by" | Clone relationship |

Only **Blocks** affects the ready queue. All other link types are informational.

---

## Global Flags (All Agent Commands)

| Flag | Type | Description |
|------|------|-------------|
| `--project` | string | Jira project key (overrides config default) |
| `--json` | bool | JSON output |
| `--jq` | string | jq filter (implies --json) |
| `--quiet` | bool | Suppress non-essential output |
| `--dry-run` | bool | Preview without mutating |
| `--text` | bool | Force text output |

These inherit from jira-cli's existing global flag system.

---

## Error Contract

All agent commands return structured errors following jira-cli's existing `CLIError` pattern:

```json
{
  "error": {
    "code": "CONFLICT_ERROR",
    "message": "Issue PROJ-123 is already assigned to john@example.com",
    "context": { "key": "PROJ-123", "assignee": "john@example.com" },
    "suggestion": "Use --force to claim anyway"
  }
}
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Auth error |
| 3 | Validation error (invalid transition, missing field) |
| 4 | Not found |
| 5 | Permission denied |
| 6 | Rate limited |
| 7 | Network error |
| 8 | Conflict (already claimed) |

---

## Session Lifecycle Contract

### Session Start
```
1. jira agent prime                    → inject workflow context (auto via hook)
2. jira agent ready --json             → find available work
3. jira issue view <key>               → review candidate in detail
4. jira agent claim <key>              → claim it
```

### Work Phase
```
- Implement the claimed issue
- jira agent discover <key> --title="Found: ..."  → file discovered work
- jira comment add <key> --body "progress note"   → breadcrumbs
```

### Session End
```
1. jira agent discover <key> --title="..." → file remaining work (if any)
2. make test && make lint                  → quality gates
3. jira agent close <key> --reason="..."   → close completed work
4. git add <files> && git commit           → commit code
5. git push                                → push to remote
```

### Context Recovery (After Compaction)
- PreCompact hook runs `jira agent prime` automatically
- Agent re-reads workflow rules and continues
