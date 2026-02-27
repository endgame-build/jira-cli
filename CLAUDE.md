# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```sh
make build          # bin/jira
make test           # go test ./...
make lint           # go vet ./...
make install        # go install ./cmd/jira
go test ./internal/api/...              # single package
go test ./internal/cmd/issue/... -run TestView  # single test
```

Ldflags inject version info: `-X main.version=... -X main.commit=... -X main.date=...`.

Pre-commit hooks run `gofmt`, `go vet`, `go build`, `go test -short`, and gitleaks on commit. Install with `make hooks`. Commit messages are enforced as conventional commits with allowed types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `build`, `ci`.

## Architecture

Go CLI for Jira Cloud REST API v3. Follows the gh-CLI blueprint: Cobra command tree, Factory DI, lazy auth.

### Layer order (no upward imports)

```
errors → iostreams → config → auth → api → output → adf → shared → factory → commands → main
```

### Factory

`factory.Factory` is the DI hub. IOStreams resolves eagerly; Config, Auth, and APIClient resolve lazily via `sync.Once`. Auth-free commands (help, config, alias, meta) skip credential resolution entirely. Global flags (`--json`, `--quiet`, `--jq`, `--text`, `--dry-run`, `--no-color`, `--profile`, `--instance`, `--user`, `--token`) are bound to Factory fields and resolved in `root.preRun` (PersistentPreRunE). `--jq` implies `--json`; `--text` overrides config-level JSON; `--json`+`--text` and `--quiet`+`--json` are rejected as conflicts.

### Command pattern

Every command follows this shape:

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

- `XxxOptions` holds all resolved inputs for `runXxx`
- No `init()`, no global state
- Commands return `error`; only `main.go` renders errors and calls `os.Exit`
- Command tree: `auth`, `issue`, `search`, `comment`, `project`, `user`, `schema`, `meta`, `config`, `alias`

### Errors

All errors use `CLIError` (code, message, context, suggestion). Codes map to exit codes 0–8. `main.go` wraps unknown errors as `GENERAL_ERROR`.

### Output

`output.Formatter` routes JSON vs text. Five shapes:
- `OutputData` — single item
- `OutputList` — list with pagination envelope
- `OutputMutation` — mutation result (`"ok": true`)
- `OutputDryRun` — dry-run preview (payload + validation)
- `OutputDryRunWithContext` — dry-run with extra context fields

`output.ApplyJQ` filters `--jq` expressions (itchyny/gojq, pure Go).

### API client

`api.Client.Do(ctx, method, path, body, out)` — single HTTP entry point. Transport chain: retryablehttp → authTransport → http.DefaultTransport. Accepts 200/201/204. Maps errors via `mapHTTPError` to `CLIError`. Retries only 429 and 5xx (skips 401, timeouts).

### ADF

`internal/adf`: Markdown → ADF (create/edit), ADF → plaintext via `ToPlaintext` (display).

### Shared utilities

`internal/cmd/shared`:
- `ValidateIssueKeyOrID` — issue key/ID validation (re-exported by `internal/cmd/issue/validate.go`)
- `ValidateProjectKeyOrID` — project key/ID validation
- `ValidateCommentID` — numeric comment ID validation
- `ReadBodyFile` — read body from file or stdin, enforces 10 MB limit
- Display helpers — comment/issue rendering with color

## Test patterns

- `factory.NewTestFactory(ios, cfg, client)` — pre-wired factory, no credential resolution
- `iostreams.Test()` — IOStreams + stdout/stderr buffers
- `httptest.NewServer` — API mocks
- `keyring.MockInit()` — in-memory keyring
- `t.TempDir()` for config files, `t.Setenv()` for env vars
- Table-driven tests throughout
- `BrowserOpen` on options structs — override in tests to capture URLs

## Conventions

- Commits: `type(scope): description` (conventional commits)
- Config writes: write-to-temp-then-rename (atomic)
- Credential chain: flags → env vars (`JIRA_INSTANCE`, `JIRA_USER`, `JIRA_TOKEN`) → stored profiles
- `--no-pager` per-command (view, list, search), not global
- `--text` overrides config-level `output.format:json` back to text
- `--web + --json` dual-action: prints JSON AND opens browser
- `--field key=value` splits on first `=`; named flag wins over `--field` collision (warning to stderr)
- All mutation JSON includes `"ok": true`
