---
name: ralph-plan
description: "Convert implementation plans to plan.json format for the Ralph autonomous agent system. Use when you have an existing implementation plan and need to convert it to Ralph's JSON format. Triggers on: convert this plan, turn this into ralph format, create plan.json from this, ralph json."
user-invocable: true
---

# Ralph Plan Converter

Converts implementation plans to the plan.json format that Ralph uses for autonomous execution.

---

## The Job

Take an implementation plan (markdown file or text) and convert it to `plan.json` in your ralph directory.

---

## Output Format

```json
{
  "project": "[Project Name]",
  "branchName": "ralph/[feature-name-kebab-case]",
  "description": "[Feature description from plan title/intro]",
  "tasks": [
    {
      "id": "T-001",
      "title": "[Task title]",
      "description": "[What needs to be done and why]",
      "type": "schema | backend | frontend | test | config | refactor",
      "complexity": "small | medium | large",
      "successCriteria": [
        "Criterion 1",
        "Criterion 2",
        "Typecheck passes"
      ],
      "dependsOn": [],
      "priority": 1,
      "passes": false,
      "notes": ""
    }
  ]
}
```

---

## Task Size: The Number One Rule

**Each task must be completable in ONE Ralph iteration (one context window).**

Ralph spawns a fresh instance per iteration with no memory of previous work. If a task is too big, the LLM runs out of context before finishing and produces broken code.

### Right-sized tasks:
- Add a database column and migration
- Add a UI component to an existing page
- Update a server action with new logic
- Add a filter dropdown to a list

### Too big (split these):
- "Build the entire dashboard" — Split into: schema, queries, UI components, filters
- "Add authentication" — Split into: schema, middleware, login UI, session handling
- "Refactor the API" — Split into one task per endpoint or pattern

**Rule of thumb:** If you cannot describe the change in 2-3 sentences, it is too big.

---

## Writing Effective Descriptions

**Ralph has zero memory between iterations.** The task description is the entire briefing for a fresh context window. Vague descriptions cause Ralph to guess wrong, explore unnecessarily, or produce code that doesn't integrate.

### Include in every description:
- **What** needs to change (the concrete action)
- **Where** in the codebase (file paths, module names, function names)
- **How** it connects (what existing code to reference or follow as a pattern)

### Weak description (Ralph will struggle):
> "Add a status column to persist task progress state in the database."

### Strong description (Ralph can execute immediately):
> "Add a `status` column to the `tasks` table. Create a new migration in `db/migrations/` following the pattern in `0001_create_tasks.sql`. Column type: text enum with values 'pending', 'in_progress', 'done', default 'pending'. Update the Task model in `src/models/task.ts` to include the new field."

### The test: Could a developer with no project context complete this task using ONLY the description? If not, add more detail.

When the implementation plan mentions specific files, functions, or patterns — carry them into the task description. This is the highest-leverage thing you can do for Ralph's success rate.

---

## Task Ordering: Dependencies First

Tasks execute in priority order. Earlier tasks must not depend on later ones. Use `dependsOn` to make dependencies explicit.

**Correct order:**
1. Schema/database changes (migrations)
2. Server actions / backend logic
3. UI components that use the backend
4. Dashboard/summary views that aggregate data

**Wrong order:**
1. UI component (depends on schema that does not exist yet)
2. Schema change

### The `type` Field

Categorize each task so Ralph knows what tools and patterns to reach for:

| Type | Description |
|------|-------------|
| `schema` | Database migrations, column additions, index changes |
| `backend` | Server actions, API routes, business logic |
| `frontend` | UI components, pages, client-side behavior |
| `test` | Test files, test utilities, fixtures. Create a separate test task when tests are non-trivial (new test infrastructure, complex fixtures, multiple test cases). For simple "does it work" verification, just add "Tests pass" as a success criterion on the feature task itself. |
| `config` | Configuration files, environment setup |
| `refactor` | Restructuring without behavior change |

### The `complexity` Field

A context budget signal. Tells Ralph how much exploration room it has:

| Complexity | Guideline |
|------------|-----------|
| `small` | < 50 lines changed, single file or tightly scoped |
| `medium` | 50–150 lines, 2–3 files, straightforward logic |
| `large` | 150+ lines or 4+ files — at the limit of one context window |

**If a task is "large", strongly consider splitting it.** Large tasks have the highest failure rate. Only use "large" when the work is genuinely indivisible.

### The `dependsOn` Field

Each task may list IDs of tasks that must complete before it can run. Ralph skips blocked tasks and returns to them once dependencies pass.

- `"dependsOn": []` — no blockers, can run immediately
- `"dependsOn": ["T-001", "T-002"]` — waits for both to pass

Priority still controls execution order among unblocked tasks. `dependsOn` adds a safety net — if a dependency fails, Ralph won't waste a context window on a task that can't succeed.

---

## Success Criteria: Must Be Verifiable

Each criterion must be something Ralph can CHECK, not something vague.

### Good criteria (verifiable):
- "Add `status` column to tasks table with default 'pending'"
- "Filter dropdown has options: All, Active, Completed"
- "Clicking delete shows confirmation dialog"
- "Typecheck passes"
- "Tests pass"

### Bad criteria (vague):
- "Works correctly"
- "User can do X easily"
- "Good UX"
- "Handles edge cases"

### Always include as final criterion:
```
"Typecheck passes"
```

For tasks with testable logic, also include:
```
"Tests pass"
```

### For tasks that change UI, also include:
```
"Verify in browser using dev-browser skill"
```

Frontend tasks are NOT complete until visually verified. Ralph will use the dev-browser skill to navigate to the page, interact with the UI, and confirm changes work.

---

## Conversion Rules

1. **Each task becomes one JSON entry**
2. **IDs**: Sequential (T-001, T-002, etc.)
3. **Priority**: Based on dependency order, then document order
4. **dependsOn**: Populate from explicit or inferred dependencies in the plan
5. **type**: Infer from the task's nature (schema, backend, frontend, test, config, refactor)
6. **complexity**: Estimate from scope — small/medium/large
7. **All tasks**: `passes: false` and empty `notes`
8. **branchName**: Derive from feature name, kebab-case, prefixed with `ralph/`
9. **project**: Use the repository or application name (e.g., from `package.json` `name`, `Cargo.toml` package name, or directory name)
10. **Always add**: "Typecheck passes" to every task's success criteria
11. **Descriptions**: Include file paths, function names, and pattern references from the plan (see "Writing Effective Descriptions")

---

## Splitting Large Plans

If a plan has big features, split them:

**Original:**
> "Add user notification system"

**Split into:**
1. T-001: Add notifications table to database
2. T-002: Create notification service for sending notifications — `dependsOn: ["T-001"]`
3. T-003: Add notification bell icon to header — `dependsOn: ["T-002"]`
4. T-004: Create notification dropdown panel — `dependsOn: ["T-003"]`
5. T-005: Add mark-as-read functionality — `dependsOn: ["T-004"]`
6. T-006: Add notification preferences page — `dependsOn: ["T-001"]`

Each is one focused change that can be completed and verified independently. Note how T-006 only depends on T-001 (the schema), not the UI chain — `dependsOn` captures the real dependency graph, not just a linear sequence.

---

## Examples

### Example 1: Simple Feature (single type chain)

**Input Implementation Plan:**
```markdown
# Task Status Feature

Add ability to mark tasks with different statuses.

## Implementation Steps
- Add status column to database
- Show status badge on each task in the list
- Add toggle control to change status inline
- Add filter dropdown to filter by status
- Persist filter selection in URL params
```

**Output plan.json:**
```json
{
  "project": "TaskApp",
  "branchName": "ralph/task-status",
  "description": "Task Status Feature — Track task progress with status indicators",
  "tasks": [
    {
      "id": "T-001",
      "title": "Add status field to tasks table",
      "description": "Create a migration in `db/migrations/` following the pattern in `0001_create_tasks.sql`. Add a `status` text column to the `tasks` table with values 'pending' | 'in_progress' | 'done', default 'pending'. Update the Task model in `src/models/task.ts` to include the new field with its type union.",
      "type": "schema",
      "complexity": "small",
      "successCriteria": [
        "Add status column: 'pending' | 'in_progress' | 'done' (default 'pending')",
        "Generate and run migration successfully",
        "Typecheck passes"
      ],
      "dependsOn": [],
      "priority": 1,
      "passes": false,
      "notes": ""
    },
    {
      "id": "T-002",
      "title": "Display status badge on task cards",
      "description": "Add a StatusBadge component in `src/components/` that renders a colored pill for each status value. Integrate it into the existing TaskCard component in `src/components/TaskCard.tsx`. Color mapping: gray=pending, blue=in_progress, green=done.",
      "type": "frontend",
      "complexity": "small",
      "successCriteria": [
        "Each task card shows colored status badge",
        "Badge colors: gray=pending, blue=in_progress, green=done",
        "Typecheck passes",
        "Verify in browser using dev-browser skill"
      ],
      "dependsOn": ["T-001"],
      "priority": 2,
      "passes": false,
      "notes": ""
    },
    {
      "id": "T-003",
      "title": "Add status toggle to task list rows",
      "description": "Add an inline status dropdown to each row in `src/components/TaskList.tsx`. On change, call the existing `updateTask` server action in `src/actions/tasks.ts` with the new status. Use optimistic UI update pattern from the existing edit-in-place title feature.",
      "type": "frontend",
      "complexity": "medium",
      "successCriteria": [
        "Each row has status dropdown or toggle",
        "Changing status saves immediately",
        "UI updates without page refresh",
        "Typecheck passes",
        "Verify in browser using dev-browser skill"
      ],
      "dependsOn": ["T-002"],
      "priority": 3,
      "passes": false,
      "notes": ""
    },
    {
      "id": "T-004",
      "title": "Filter tasks by status",
      "description": "Add a filter dropdown above the task list in `src/components/TaskList.tsx`. Read/write the `status` URL search param using the existing `useQueryParams` hook in `src/hooks/`. Filter the task query in `src/actions/tasks.ts` by adding an optional `status` WHERE clause.",
      "type": "frontend",
      "complexity": "medium",
      "successCriteria": [
        "Filter dropdown: All | Pending | In Progress | Done",
        "Filter persists in URL params",
        "Typecheck passes",
        "Verify in browser using dev-browser skill"
      ],
      "dependsOn": ["T-001"],
      "priority": 4,
      "passes": false,
      "notes": ""
    }
  ]
}
```

### Example 2: Mixed types with diamond dependency

**Input Implementation Plan:**
```markdown
# API Rate Limiting

Add per-user rate limiting to the public API.

## Implementation Steps
- Add rate_limits table to track request counts
- Add rate limit middleware that checks/increments counts
- Add rate limit headers to API responses (X-RateLimit-Remaining, etc.)
- Add admin endpoint to view/reset rate limits
- Add integration tests for rate limit enforcement
```

**Output plan.json:**
```json
{
  "project": "TaskApp",
  "branchName": "ralph/api-rate-limiting",
  "description": "API Rate Limiting — Per-user request throttling for the public API",
  "tasks": [
    {
      "id": "T-001",
      "title": "Add rate_limits table",
      "description": "Create a migration in `db/migrations/` for a `rate_limits` table: columns `user_id` (FK to users), `endpoint` (text), `window_start` (timestamp), `request_count` (integer, default 0). Add composite unique index on (user_id, endpoint, window_start). Add the RateLimit model in `src/models/rateLimit.ts` following the pattern in `src/models/user.ts`.",
      "type": "schema",
      "complexity": "small",
      "successCriteria": [
        "rate_limits table created with user_id, endpoint, window_start, request_count columns",
        "Composite unique index on (user_id, endpoint, window_start)",
        "Migration runs successfully",
        "Typecheck passes"
      ],
      "dependsOn": [],
      "priority": 1,
      "passes": false,
      "notes": ""
    },
    {
      "id": "T-002",
      "title": "Add rate limit middleware",
      "description": "Create `src/middleware/rateLimit.ts` that checks the rate_limits table for the current user+endpoint+window. If count >= limit (default 100/hour), return 429. Otherwise increment count. Plug into the middleware chain in `src/app.ts` after the auth middleware. Use the existing `getAuthUser()` helper from `src/middleware/auth.ts` to get the user ID.",
      "type": "backend",
      "complexity": "medium",
      "successCriteria": [
        "Middleware checks rate_limits table per user+endpoint",
        "Returns 429 when limit exceeded",
        "Increments count on each request",
        "Typecheck passes"
      ],
      "dependsOn": ["T-001"],
      "priority": 2,
      "passes": false,
      "notes": ""
    },
    {
      "id": "T-003",
      "title": "Add rate limit response headers",
      "description": "Extend the rate limit middleware in `src/middleware/rateLimit.ts` to set response headers: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset` (Unix timestamp of window end). Headers should be added on ALL responses (not just 429s) so clients can track their usage.",
      "type": "backend",
      "complexity": "small",
      "successCriteria": [
        "All API responses include X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset headers",
        "Header values are accurate against the rate_limits table",
        "Typecheck passes"
      ],
      "dependsOn": ["T-002"],
      "priority": 3,
      "passes": false,
      "notes": ""
    },
    {
      "id": "T-004",
      "title": "Add admin rate limit management endpoint",
      "description": "Add GET `/api/admin/rate-limits` and DELETE `/api/admin/rate-limits/:userId` routes in `src/routes/admin.ts`. GET returns current counts for all users. DELETE resets a user's counts. Guard both with the existing `requireAdmin` middleware from `src/middleware/auth.ts`.",
      "type": "backend",
      "complexity": "medium",
      "successCriteria": [
        "GET /api/admin/rate-limits returns all user rate limit records",
        "DELETE /api/admin/rate-limits/:userId resets that user's counts",
        "Both endpoints require admin auth",
        "Typecheck passes"
      ],
      "dependsOn": ["T-002"],
      "priority": 4,
      "passes": false,
      "notes": ""
    },
    {
      "id": "T-005",
      "title": "Add rate limit integration tests",
      "description": "Create `tests/rateLimit.test.ts` following the test pattern in `tests/auth.test.ts`. Test cases: request under limit returns 200 with correct headers, request at limit returns 429, count resets after window expires, admin reset clears counts. Use the existing `createTestUser` and `asAdmin` helpers from `tests/helpers.ts`.",
      "type": "test",
      "complexity": "medium",
      "successCriteria": [
        "Tests cover: under-limit, at-limit (429), window reset, admin reset",
        "All tests pass",
        "Typecheck passes"
      ],
      "dependsOn": ["T-003", "T-004"],
      "priority": 5,
      "passes": false,
      "notes": ""
    }
  ]
}
```

Note the **diamond dependency**: T-003 and T-004 both depend on T-002, then T-005 depends on both T-003 and T-004. This captures the real graph — tests can't run until both the headers and admin endpoint exist.

---

## Archiving Previous Runs

**Before writing a new plan.json, check if there is an existing one from a different feature:**

1. Read the current `plan.json` if it exists
2. Check if `branchName` differs from the new feature's branch name
3. If different AND `PROGRESS.md` has content beyond the header:
   - Create archive folder: `archive/YYYY-MM-DD-feature-name/`
   - Copy current `plan.json` and `PROGRESS.md` to archive
   - Reset `PROGRESS.md` with fresh header

**The ralph.sh script handles this automatically** when you run it, but if you are manually updating plan.json between runs, archive first.

---

## Checklist Before Saving

Before writing plan.json, verify:

- [ ] **Previous run archived** (if plan.json exists with different branchName, archive it first)
- [ ] Each task is completable in one iteration (small enough)
- [ ] No task marked "large" unless genuinely indivisible
- [ ] Tasks are ordered by dependency (schema → backend → UI)
- [ ] `dependsOn` accurately reflects real dependencies
- [ ] No circular dependencies in `dependsOn`
- [ ] Every task has a `type` and `complexity`
- [ ] Every task has "Typecheck passes" as criterion
- [ ] UI tasks have "Verify in browser using dev-browser skill" as criterion
- [ ] Success criteria are verifiable (not vague)
- [ ] Descriptions include file paths and pattern references (not just "what")
