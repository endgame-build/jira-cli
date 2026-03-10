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

**Before generating:** Read `ralph/RALPH.md` and `CLAUDE.md` to understand project conventions, build commands, file structure, and test patterns. Task descriptions must use the actual project's terminology and file paths.

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
      "type": "backend | test | config | refactor | [project-specific types]",
      "complexity": "small | medium | large",
      "successCriteria": [
        "Criterion 1",
        "Criterion 2",
        "Build passes"
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
- Add a new type/struct and its methods
- Implement a single command or endpoint
- Add tests for one module
- Add a utility function to an existing package

### Too big (split these):
- "Build the entire feature" — Split into: types, core logic, commands/routes, tests
- "Add authentication" — Split into: config, storage, transport/middleware, commands
- "Refactor the API layer" — Split into one task per method or pattern

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
> "Add a `status` column to the `tasks` table. Create a new migration in `db/migrations/` following the pattern in `0001_create_tasks.sql`. Column type: text enum with values 'pending', 'in_progress', 'done', default 'pending'. Update the Task model in the models module to include the new field."

### The test: Could a developer with no project context complete this task using ONLY the description? If not, add more detail.

When the implementation plan mentions specific files, functions, or patterns — carry them into the task description. This is the highest-leverage thing you can do for Ralph's success rate.

---

## Task Ordering: Dependencies First

Tasks execute in priority order. Earlier tasks must not depend on later ones. Use `dependsOn` to make dependencies explicit.

**Correct order:** lower-level dependencies before higher-level consumers:
1. Types, models, schemas
2. Core logic, services, API clients
3. Commands, routes, controllers that use the core
4. Integration tests that exercise the full stack

**Wrong order:**
1. Command that calls a service method (depends on code that doesn't exist yet)
2. Service method

### The `type` Field

Categorize each task so Ralph knows what tools and patterns to reach for. Use types that match the project — common examples:

| Type | Description |
|------|-------------|
| `backend` | Core logic, API routes, services, business rules |
| `test` | Test files, test utilities, fixtures. Create a separate test task when tests are non-trivial (new test infrastructure, complex fixtures, multiple test cases). For simple "does it work" verification, just add "Tests pass" as a success criterion on the feature task itself. |
| `config` | Configuration files, environment setup, build changes |
| `refactor` | Restructuring without behavior change |

Add project-specific types as needed (e.g., `schema` for DB migrations, `frontend` for UI, `cli` for command-line commands).

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
- "Build passes"
- "Tests pass"

### Bad criteria (vague):
- "Works correctly"
- "User can do X easily"
- "Good UX"
- "Handles edge cases"

### Always include as final criterion:
```
"Build passes"
```

For tasks with testable logic, also include:
```
"Tests pass"
```

### For tasks with visible output (UI, CLI display, reports), also include verification criteria appropriate to the project:
- UI tasks: "Verify in browser using dev-browser skill"
- CLI tasks: "Verify output matches expected format"
- Report tasks: "Verify generated output is correct"

---

## Conversion Rules

1. **Each task becomes one JSON entry**
2. **IDs**: Sequential (T-001, T-002, etc.)
3. **Priority**: Based on dependency order, then document order
4. **dependsOn**: Populate from explicit or inferred dependencies in the plan
5. **type**: Infer from the task's nature (backend, test, config, refactor, or project-specific types)
6. **complexity**: Estimate from scope — small/medium/large
7. **All tasks**: `passes: false` and empty `notes`
8. **branchName**: Derive from feature name, kebab-case, prefixed with `ralph/`
9. **project**: Use the repository or application name (e.g., from `package.json`, `Cargo.toml`, `go.mod`, or directory name)
10. **Always add**: "Build passes" to every task's success criteria
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

### Example 1: Linear dependency chain

**Input Implementation Plan:**
```markdown
# Export Feature

Add ability to export items to markdown files.

## Implementation Steps
- Add markdown conversion utility
- Add file-writing helper with atomic writes
- Add export command with --dry-run and --limit flags
- Add tests for export command
```

**Output plan.json:**
```json
{
  "project": "MyApp",
  "branchName": "ralph/export-feature",
  "description": "Export Feature — Export items to markdown files",
  "tasks": [
    {
      "id": "T-001",
      "title": "Add markdown conversion utility",
      "description": "Create a `toMarkdown()` function in the output/formatting module that converts internal data structures to markdown. Follow the pattern in the existing `toJSON()` formatter. Handle all field types: strings, dates, nested objects, lists.",
      "type": "backend",
      "complexity": "small",
      "successCriteria": [
        "toMarkdown() converts all field types correctly",
        "Nil/empty input returns empty string",
        "Build passes"
      ],
      "dependsOn": [],
      "priority": 1,
      "passes": false,
      "notes": ""
    },
    {
      "id": "T-002",
      "title": "Add file-writing helper with atomic writes",
      "description": "Create a `writeFileAtomic()` helper that writes to a temp file then renames. Place it alongside existing file utilities. Accept output directory and filename as parameters. Create parent directories as needed.",
      "type": "backend",
      "complexity": "small",
      "successCriteria": [
        "Writes use temp-then-rename pattern",
        "Parent directories created automatically",
        "Build passes"
      ],
      "dependsOn": [],
      "priority": 2,
      "passes": false,
      "notes": ""
    },
    {
      "id": "T-003",
      "title": "Add export command",
      "description": "Create the export command following the project's command pattern. Flags: --output-dir (default '.'), --dry-run (list files without writing), --limit (max items). Use toMarkdown() from T-001 and writeFileAtomic() from T-002. Report progress to stderr.",
      "type": "backend",
      "complexity": "medium",
      "successCriteria": [
        "Export command registered with correct flags and help text",
        "--dry-run lists files without writing",
        "--limit stops at specified count",
        "Progress reported to stderr",
        "Build passes"
      ],
      "dependsOn": ["T-001", "T-002"],
      "priority": 3,
      "passes": false,
      "notes": ""
    },
    {
      "id": "T-004",
      "title": "Add tests for export command",
      "description": "Create tests following the project's existing test patterns. Cover: basic export writes correct files, --dry-run prevents writes, --limit caps output, empty input handled gracefully. Use existing test helpers for mocking.",
      "type": "test",
      "complexity": "medium",
      "successCriteria": [
        "Tests cover: basic export, dry-run, limit, empty input",
        "All tests pass",
        "Build passes"
      ],
      "dependsOn": ["T-003"],
      "priority": 4,
      "passes": false,
      "notes": ""
    }
  ]
}
```

### Example 2: Diamond dependency

**Input Implementation Plan:**
```markdown
# Retry with Backoff

Add configurable retry logic with exponential backoff to the HTTP client.

## Implementation Steps
- Add retry configuration types
- Add backoff calculator
- Add retry middleware/wrapper for HTTP calls
- Add circuit breaker to stop retrying after threshold
- Add integration tests
```

**Output plan.json:**
```json
{
  "project": "MyApp",
  "branchName": "ralph/retry-backoff",
  "description": "Retry with Backoff — Configurable retry logic for HTTP client",
  "tasks": [
    {
      "id": "T-001",
      "title": "Add retry configuration types",
      "description": "Add RetryConfig struct/type with fields: maxRetries (int, default 3), baseDelay (duration, default 1s), maxDelay (duration, default 30s), retryableStatuses (list of HTTP codes, default [429, 500, 502, 503]). Place in the HTTP client module alongside existing transport types.",
      "type": "backend",
      "complexity": "small",
      "successCriteria": [
        "RetryConfig type with all fields and sensible defaults",
        "Build passes"
      ],
      "dependsOn": [],
      "priority": 1,
      "passes": false,
      "notes": ""
    },
    {
      "id": "T-002",
      "title": "Add backoff calculator",
      "description": "Add a `calculateBackoff(attempt, config)` function that returns the delay for a given attempt using exponential backoff with jitter. Formula: min(baseDelay * 2^attempt + random_jitter, maxDelay). Place alongside RetryConfig from T-001.",
      "type": "backend",
      "complexity": "small",
      "successCriteria": [
        "Exponential backoff with jitter",
        "Respects maxDelay cap",
        "Build passes"
      ],
      "dependsOn": ["T-001"],
      "priority": 2,
      "passes": false,
      "notes": ""
    },
    {
      "id": "T-003",
      "title": "Add retry wrapper for HTTP calls",
      "description": "Add a retry wrapper/middleware that wraps the existing HTTP transport. On retryable status codes, wait using calculateBackoff() from T-002, then retry. Pass through non-retryable responses immediately. Log retry attempts to debug output.",
      "type": "backend",
      "complexity": "medium",
      "successCriteria": [
        "Retries on configured status codes",
        "Uses exponential backoff between retries",
        "Passes through non-retryable responses immediately",
        "Build passes"
      ],
      "dependsOn": ["T-002"],
      "priority": 3,
      "passes": false,
      "notes": ""
    },
    {
      "id": "T-004",
      "title": "Add circuit breaker",
      "description": "Add a circuit breaker that wraps the HTTP transport independently of the retry wrapper. After N consecutive failures (configurable, default 5), return errors immediately for a cooldown period. Add `circuitBreakerThreshold` and `cooldownPeriod` fields to RetryConfig from T-001. The retry wrapper (T-003) and circuit breaker compose as separate layers.",
      "type": "backend",
      "complexity": "medium",
      "successCriteria": [
        "Stops retrying after consecutive failure threshold",
        "Returns errors immediately during cooldown",
        "Resumes retrying after cooldown expires",
        "Build passes"
      ],
      "dependsOn": ["T-002"],
      "priority": 4,
      "passes": false,
      "notes": ""
    },
    {
      "id": "T-005",
      "title": "Add retry integration tests",
      "description": "Create tests using a mock HTTP server. Test cases: successful retry after transient error, max retries exceeded returns last error, non-retryable status passes through, circuit breaker opens after threshold, circuit breaker resets after cooldown. Follow existing test patterns in the project.",
      "type": "test",
      "complexity": "medium",
      "successCriteria": [
        "Tests cover: successful retry, max retries, non-retryable passthrough, circuit breaker open/reset",
        "All tests pass",
        "Build passes"
      ],
      "dependsOn": ["T-003", "T-004"],
      "priority": 5,
      "passes": false,
      "notes": ""
    }
  ]
}
```

Note the **diamond dependency**: T-003 and T-004 both depend on T-002 (diverge), then T-005 depends on both T-003 and T-004 (converge). `dependsOn` captures the real dependency graph, not just a linear sequence.

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
- [ ] Tasks are ordered by dependency (lower-level before higher-level consumers)
- [ ] `dependsOn` accurately reflects real dependencies
- [ ] No circular dependencies in `dependsOn`
- [ ] Every task has a `type` and `complexity`
- [ ] Every task has "Build passes" as criterion
- [ ] Tasks with visible output have appropriate verification criteria
- [ ] Success criteria are verifiable (not vague)
- [ ] Descriptions include file paths and pattern references (not just "what")
