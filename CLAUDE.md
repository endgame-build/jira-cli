# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```sh
make build          # Build binary to bin/jira
make test           # Run all tests (go test ./...)
make lint           # Run go vet ./...
make install        # go install ./cmd/jira
go test ./internal/api/...              # Run tests for a single package
go test ./internal/cmd/issue/... -run TestView  # Run a specific test
```

Version info is injected via ldflags (`-X main.version=... -X main.commit=... -X main.date=...`).

## Architecture

This is a Go CLI wrapping Jira Cloud REST API v3, following the **gh-CLI blueprint**: Cobra command tree, Factory DI container, lazy auth resolution.

### Dependency flow (strict layering — no upward imports)

```
errors → iostreams → config → auth → api → output → adf → factory → commands → main
```

### Factory DI pattern

`factory.Factory` is the single DI hub. IOStreams is eager; Config, Auth, and APIClient are lazy via `sync.Once`. Auth-free commands (help, config, alias) never trigger credential resolution.

### Command constructor pattern

Every command follows this exact shape:

```go
func NewCmdXxx(f *factory.Factory) *cobra.Command {
    opts := &XxxOptions{Factory: f}
    cmd := &cobra.Command{
        RunE: func(cmd *cobra.Command, args []string) error {
            // validate args, resolve lazy deps
            return runXxx(opts)
        },
    }
    // register flags
    return cmd
}
```

- `XxxOptions` struct holds all resolved inputs for `runXxx`
- No `init()` functions, no global state
- Commands return `error` — only `main.go` renders errors and calls `os.Exit`

### Error handling

All errors are `CLIError` (structured with code, message, context, suggestion). Error codes map to exit codes 0-8. `main.go` is the sole error renderer — it wraps unknown errors as `GENERAL_ERROR`.

### Output system

`output.Formatter` routes between JSON and text modes. Three output shapes:
- `OutputData` — single item (view commands)
- `OutputList` — list with pagination envelope
- `OutputMutation` — mutation result with `"ok": true`

`--jq` expressions applied via `output.ApplyJQ` (itchyny/gojq, pure Go).

### API client

`api.Client.Do(ctx, method, path, body, out)` handles all HTTP. Transport chain: retryablehttp → authTransport → http.DefaultTransport. Accepts 200/201/204; maps errors via `mapHTTPError` to `CLIError`. Never retries 401 or timeouts; only 429 and 5xx.

### ADF converter

`internal/adf` converts Markdown → Atlassian Document Format (for creating/editing issues) and ADF → plaintext (for displaying).

## Test patterns

- `factory.NewTestFactory(ios, cfg, client)` — pre-wired factory, no credential resolution
- `iostreams.Test()` — returns IOStreams + stdout/stderr buffers for capture
- `httptest.NewServer` — API mock servers in test files
- `keyring.MockInit()` — in-memory keyring for credential tests
- `t.TempDir()` for config files, `t.Setenv()` for env vars
- Table-driven tests throughout
- `BrowserOpen` field on options structs overridden in tests to capture URLs

## Key conventions

- Commits: `type(scope): description` (conventional commits)
- Config writes use write-to-temp-then-rename for atomicity
- Credential chain: flags → env vars (`JIRA_INSTANCE`, `JIRA_USER`, `JIRA_TOKEN`) → stored profiles
- `--no-pager` is per-command (view, list, search), not global
- `--web + --json` is dual-action: prints JSON AND opens browser
- `--field key=value` splits on first `=` only; named flag wins over `--field` collision (warning to stderr)
- All mutation JSON outputs include `"ok": true`
- Issue key validation via `ValidateIssueKeyOrID` in `internal/cmd/issue/validate.go`

## Reference docs

- `SPEC.md` — functional specification
- `PLAN.md` — implementation plan with architecture
- `tasks/prd-jira-cli-foundation.md` — PRD for Phases 0-2 (32 user stories)
- `swagger-v3.v3.txt` — Jira Cloud v3 OpenAPI spec (2.46MB reference)
- PRD supersedes SPEC.md and PLAN.md where they differ
