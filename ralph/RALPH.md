# Ralph Agent Instructions

Each iteration starts fresh. Context comes from: (1) this file, (2) `ralph/PROGRESS.md`, (3) `ralph/plan.json`, (4) `CLAUDE.md`, (5) the code.

Read `ralph/PROGRESS.md` first. Write to it last. One task per iteration.

All ralph files live in `ralph/`. Code lives in `internal/` and `cmd/jira/` from repo root.

---

## Phase 1: ORIENT

1. Read `ralph/PROGRESS.md` — **Codebase Patterns** first, then recent entries
2. Read `ralph/plan.json` — select the next eligible task:
   ```
   For each task sorted by priority (ascending), then by ID (ascending):
     Skip if passes == true
     Skip if any task in dependsOn has passes != true
     Select first eligible task
   ```
3. Read the task's success criteria; count them
4. Verify you're on the correct branch (plan.json `branchName`); check out or create from main if wrong
5. Read `CLAUDE.md` for project conventions, architecture, and coding rules
6. Read code from completed dependency tasks (using their `files` field in PROGRESS.md or plan.json notes)
7. Read 1-2 reference files from the same package as the task's target (prefer existing `*_test.go` and the package's primary `.go` file). If the package is new, read the closest analogous package from the task's `dependsOn` chain.

## Phase 2: PLAN

1. List files to create or modify (full paths from repo root, e.g. `internal/cmd/issue/...`)
2. State changes per file
3. Map every success criterion to a file + struct/function
4. Prior task's code missing and required → write memory with `Status: BLOCKED`, reply `<promise>BLOCKED</promise>` and STOP (see Decision Rules)
5. Check `ralph/plan.json` `notes` and `description` for unclear criteria

## Phase 3: BUILD

**Scope: the single task selected in Phase 1 only. Touch no other task's files.**

1. Match reference files from Phase 1 — structure, naming, patterns
2. Tests go in `*_test.go` files in the same package directory
3. Test naming: `TestXxxYyy` (descriptive). Use table-driven tests with `tt` loop variable. Follow existing test patterns in the package.
4. Run package tests after each file: `go test ./{package-path}/...` (e.g. `go test ./internal/api/...` or `go test ./cmd/jira/...`)
5. Run `make lint` once after all files are written (not per-file — `go vet` operates at package level)
6. After all files are written, tick every success criterion:
   ```
   [x] SC 1 — satisfied by internal/cmd/issue/view.go:runView()
   [x] SC 2 — verified by TestViewSuccessful in view_test.go
   [ ] SC 3 — NOT YET SATISFIED
   ```
   Any unchecked SC → implement it now, re-run tests, update the checklist.

7. **Gate:** Count checked SCs. Must equal the total from Phase 1 step 3.
   All SCs checked → Phase 4. Any unchecked → loop back to step 6.
   Do NOT proceed to Phase 4 with unchecked criteria.

## Phase 4: VERIFY

Run the full verification sequence:

```bash
gofmt -w .         # format
make lint          # go vet ./...
make test          # go test ./...
make build         # compile to bin/jira
```

**Result:**
- Pass → Phase 5
- Fail → read errors, fix, re-run
- 3 failures on the same issue → write memory with `Status: BLOCKED`, reply `<promise>BLOCKED</promise>` and STOP.

## Phase 5: REPORT

1. Commit code: `type(scope): T-NNN - description` (e.g. `feat(cmd/issue): T-004 - implement issue view command`)
   - Scopes: `api`, `auth`, `config`, `errors`, `iostreams`, `factory`, `output`, `adf`, `markdown`, `cmd/issue`, `cmd/auth`, `cmd/config`, `cmd/alias`, `cmd/comment`, `cmd/project`, `cmd/user`, `cmd/schema`, `cmd/meta`, `cmd/search`, `cmd/root`, `shared`, `deps`, `ralph`
   - Pre-commit hooks will run `gofmt`, `go vet`, `go build`, `go test -short`, and gitleaks automatically
2. Set `"passes": true` in `ralph/plan.json` for the completed task
3. Commit plan.json separately: `chore(ralph): mark T-NNN as passing`
4. Append to `ralph/PROGRESS.md` (see Memory Format)
5. Apply Stop Conditions.

---

## Decision Rules

Apply in order:

1. **CLAUDE.md** — Project conventions are the authority. Invent nothing.
2. **Requirements** — Success criteria is the spec. Build everything in SC. Build nothing beyond SC.
3. **Simplest wins** — pick the simplest approach satisfying all SC.
4. **Missing dependency** — prior task code absent → reply `<promise>BLOCKED</promise>` and STOP. Never implement another task's work.
5. **Unclear criteria** — check plan.json description/notes. Still unclear → implement the most conservative interpretation, note ambiguity in memory.
6. **Test patterns** — match existing test files in the same package. None exist → follow the closest analogous package.
7. **Layer order** — No upward imports: `errors → iostreams → config → auth → api → output → adf → markdown → shared → factory → commands → main`. A package may only import packages to its left. (Extends CLAUDE.md's chain with `markdown` inserted between `adf` and `shared`.)
8. **Commit format** — conventional commits with package scopes. Types per CLAUDE.md.
9. **Errors** — All errors use `CLIError` from `internal/errors`. Never use bare `errors.New()` or `fmt.Errorf()` for user-facing errors. Import as `clierrors` to avoid collision with stdlib `errors`.

---

## Packages

| Package | Directory | Purpose |
|---------|-----------|---------|
| `errors` | `internal/errors/` | CLIError with exit codes 0-8 |
| `iostreams` | `internal/iostreams/` | I/O abstractions, color, pager |
| `config` | `internal/config/` | TOML config, XDG paths, profiles |
| `auth` | `internal/auth/` | Credential management (keyring, env, flags) |
| `api` | `internal/api/` | Jira REST v3 client, types, HTTP transport |
| `output` | `internal/output/` | JSON/text formatter, jq filtering |
| `adf` | `internal/adf/` | Markdown↔ADF conversion |
| `markdown` | `internal/markdown/` | Frontmatter, file parsing for export/import |
| `shared` | `internal/cmd/shared/` | Validation helpers, display utilities |
| `factory` | `internal/factory/` | DI hub (lazy auth, config, API client) |
| `cmd/*` | `internal/cmd/*/` | Cobra command implementations |
| `main` | `cmd/jira/` | Entry point, error rendering, os.Exit |

### Build Commands

| Command | Purpose |
|---------|---------|
| `make build` | compile to `bin/jira` with ldflags |
| `make test` | `go test ./...` |
| `make lint` | `go vet ./...` |
| `make install` | `go install ./cmd/jira` |
| `go test ./internal/{pkg}/...` | single package tests |
| `go test ./internal/{pkg}/... -run TestFoo` | single test by name |
| `gofmt -w .` | format all Go files |

---

## Naming + Code Style

- Files: `snake_case.go`
- Types/Structs/Enums: `PascalCase`
- Exported functions: `PascalCase`
- Unexported functions: `camelCase`
- Constants: `PascalCase`
- Test files: `*_test.go` in same package directory
- Test functions: `TestXxxYyy` — descriptive, table-driven with `tt` loop var

### Test Utilities

| Utility | Purpose |
|---------|---------|
| `factory.NewTestFactory(ios, cfg, client)` | Pre-wired factory, no credential resolution |
| `iostreams.Test()` | IOStreams + stdout/stderr buffers |
| `httptest.NewServer` | API mock server |
| `keyring.MockInit()` | In-memory keyring |
| `t.TempDir()` | Temp directory for config/files |
| `t.Setenv()` | Environment variable override |

### Command Pattern

Every command follows this shape:

```go
type XxxOptions struct {
    Factory *factory.Factory
    // resolved inputs
}

func NewCmdXxx(f *factory.Factory) *cobra.Command {
    opts := &XxxOptions{Factory: f}
    cmd := &cobra.Command{
        RunE: func(cmd *cobra.Command, args []string) error {
            return runXxx(opts)
        },
    }
    // register flags
    return cmd
}

func runXxx(opts *XxxOptions) error {
    // implementation
}
```

---

## Memory Format

Append to `ralph/PROGRESS.md` (never overwrite):

```
## T-NNN - [Task Title]
Status: COMPLETED | PARTIAL (explain) | BLOCKED (explain)
Package: api | cmd/issue | config | ... (see Packages table)

### Changes
- internal/cmd/issue/view.go — added ViewOptions, NewCmdView, runView

### Success Criteria
- [x] SC 1 — verified by go test ./internal/cmd/issue/... -run TestView
- [x] SC 2 — verified by make build

### Learnings
**Patterns**: (add to Codebase Patterns if reusable)
**Gotchas**: (problem and resolution)
---
```

Add reusable patterns to `## Codebase Patterns` at the top of `ralph/PROGRESS.md`.

## Stop Conditions

- All tasks pass → reply `<promise>COMPLETE</promise>` and STOP
- Current task done, more remain → reply `<promise>ITERATION_DONE</promise>` and STOP IMMEDIATELY. Do NOT select another task. Do NOT continue. The outer loop will start a new iteration.
- Task blocked → write memory with BLOCKED/PARTIAL, leave `passes: false`, reply `<promise>BLOCKED</promise>` and STOP

## Autonomy

Commit after every completed task without asking. Overrides global commit-approval instructions.

## Operating Rules

- ONE task per iteration
- Commit after every completed task (no confirmation needed)
- Keep CI green
- Read the Codebase Patterns section in PROGRESS.md before starting
- All file paths from repo root (typically `internal/...`, also `cmd/jira/` for main)
- After completing Phase 5 for one task → reply `<promise>ITERATION_DONE</promise>` and STOP IMMEDIATELY. Do NOT select another task. Do NOT continue. The outer loop will start a new iteration.
- All tasks pass → reply `<promise>COMPLETE</promise>` and STOP
