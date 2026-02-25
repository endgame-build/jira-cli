# CLAUDE.md

## Project

Go CLI wrapping Jira Cloud REST API v3. Module: `github.com/endgameio/jira-cli`. Go 1.26+.

## Git Conventions

- Use [Conventional Commits](https://www.conventionalcommits.org/): `type(scope): description`
- Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `build`, `ci`
- Scope is optional but encouraged (e.g., `feat(auth): add login command`)
- Don't commit without approval

## Build & Test

```sh
make build      # outputs bin/jira
make test       # go test ./...
make lint       # go vet ./...
make install    # go install ./cmd/jira
make clean      # rm -rf bin/
```

## Architecture

gh-CLI blueprint: Cobra command tree, Factory DI, lazy auth resolution.

**Dependency order** (acyclic — build leaf-first):
```
errors → iostreams → config → auth → api → output → factory → commands → main.go
```

**Key packages:**
- `internal/errors` — CLIError type (codes, context, suggestions, exit codes 0-8)
- `internal/iostreams` — IO abstraction (TTY detection, color, pager)
- `internal/config` — TOML config at XDG path, profiles
- `internal/auth` — Keyring storage, credential resolver (flags → env → profile)
- `internal/api` — HTTP client, auth transport, retry, Jira types, typed API methods
- `internal/output` — Formatter (OutputData/OutputList/OutputMutation/OutputDryRun), table, JSON envelopes, jq
- `internal/adf` — Markdown → Atlassian Document Format converter
- `internal/factory` — Lazy DI hub (IOStreams, Config, Auth, APIClient)
- `internal/cmd/*` — Cobra command implementations

## Command Pattern

Every command follows this structure — no exceptions:

```go
func NewCmdXxx(f *factory.Factory) *cobra.Command {
    opts := &XxxOptions{}
    cmd := &cobra.Command{
        Use:   "xxx <required-arg>",
        Short: "One-line description",
        RunE: func(cmd *cobra.Command, args []string) error {
            // parse positional args into opts
            return xxxRun(cmd.Context(), f, opts)
        },
    }
    cmd.Flags().StringVar(&opts.Field, "field", "", "description")
    return cmd
}
```

- Constructor returns `*cobra.Command`, takes `*factory.Factory`
- Options struct holds parsed flags and args
- Run function takes `(context.Context, *factory.Factory, *XxxOptions)`, returns `error`
- No `init()`, no global state, no package-level vars

## Error Handling

- Commands **return** errors — they never write to stderr directly
- `main.go` is the sole error renderer (prevents double-printing)
- All errors should be `*CLIError` with code, message, context, suggestion
- Use constructors: `NewAuthError`, `NewValidationError`, `NewNotFoundError`, etc.
- Exit codes: 0=success, 1=general, 2=auth, 3=validation, 4=not-found, 5=permission, 6=rate-limited, 7=network-error, 8=conflict

## Output Rules

- Data to stdout, errors/warnings to stderr
- No ANSI escape codes when stdout is not a TTY
- JSON envelopes: bare object (single reads), `{"data":[], "pagination":{}}` (lists), `{"ok":true, ...}` (mutations)
- `OutputError` is a standalone function, not on Formatter

## API Client Rules

- All requests go through `Client.Do()` — never construct raw HTTP requests in commands
- Auth injected via `http.RoundTripper` (Basic auth: `base64(email:token)`)
- Accept 200, 201, 204 as success; skip body decode on 204
- Retry only 429 (with Retry-After) and 5xx; never retry 401 or timeouts
- Base URL: `https://{instance}/rest/api/3/{path}`

## Testing

- `httptest.NewServer` for API mocks — no real Jira instance in tests
- `iostreams.Test()` for output capture (returns buffers, `isTTY: false`)
- `keyring.MockInit()` for in-memory keyring — no OS keyring in tests
- `t.TempDir()` for config file isolation
- `t.Setenv()` for env var tests
- Table-driven tests preferred

## Dependencies (11)

| Package | Purpose |
|---------|---------|
| `spf13/cobra` | CLI framework |
| `hashicorp/go-retryablehttp` | HTTP retry with backoff |
| `BurntSushi/toml` | Config format |
| `zalando/go-keyring` | Secure credential storage |
| `yuin/goldmark` | Markdown parsing for ADF conversion |
| `jedib0t/go-pretty/v6` | Table output |
| `fatih/color` | Color output |
| `mattn/go-isatty` | TTY detection |
| `adrg/xdg` | XDG config paths |
| `cli/browser` | Cross-platform browser opening |
| `itchyny/gojq` | Pure Go jq for `--jq` flag |

## Key Conventions

- Jira API v3 only — use new endpoints (`POST /search/jql`, not `GET /search`)
- `User.EmailAddress` is `*string` (nullable — Jira privacy settings)
- `deleteSubtasks` is a string query param (`"true"`), not boolean
- Instance URLs stored as bare hostname (`mycompany.atlassian.net`)
- Config writes use write-to-temp-then-rename (atomic)
- Auth-free commands (`config`, `alias`, `--help`, `--version`) must never trigger credential resolution

## Reference Docs

- `SPEC.md` — functional specification
- `PLAN.md` — implementation plan with architecture
- `tasks/prd-jira-cli-foundation.md` — detailed PRD for Phases 0-2 (32 user stories, **supersedes SPEC.md and PLAN.md where they differ**)
