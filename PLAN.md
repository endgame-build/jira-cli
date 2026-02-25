# Jira CLI — Go Implementation Plan

## Context

Build a CLI tool (`jira`) that wraps the Jira Cloud Platform REST API v3 for developers, LLM agents, and CI/CD automation. The tool must be strictly non-interactive, pipe-safe, agent-discoverable (via `jira meta commands` + `jira schema *`), and provide structured errors for self-correction. This is a greenfield project — only `SPEC.md` exists today.

> **Note:** Where this plan and the PRD (`tasks/prd-jira-cli-foundation.md`) differ, the PRD supersedes. Key differences: Search promoted from Phase 3 to Phase 2; additional error constructors (`NewNetworkError`, `NewConflictError`, `NewAmbiguousUserError`); exit codes 7+8 added; `--confirm` renamed to `--yes`; `--jq`, `--quiet`, `--text`, `--sort`/`--order` flags added; `issue move` gains `--resolution`/`--comment`; `itchyny/gojq` added as 11th dependency; Factory gains `Quiet`/`JQExpr` fields; `OutputMutation` method added to Formatter; `OutputError` is standalone function not Formatter method.

---

## Technology Stack

| Concern | Choice | Rationale |
|---------|--------|-----------|
| CLI framework | `spf13/cobra` | Best introspection for `meta commands` (walks command tree), PersistentPreRunE hooks for auth, PersistentFlags for globals. `gh` CLI is the proven blueprint. |
| HTTP client | `net/http` + `hashicorp/go-retryablehttp` | Full control over auth middleware, rate limit handling (Retry-After), structured errors. Existing Jira Go libs lack v3 support. |
| Config format | TOML via `BurntSushi/toml` | Multi-profile config, human-editable, comments support |
| Credentials | `zalando/go-keyring` | macOS Keychain / Linux Secret Service / Windows Credential Manager. Fallback to config file with warning. |
| Markdown to ADF | Custom converter via `yuin/goldmark` | No production Go library exists. ~300-400 LOC. Parse CommonMark AST, walk it, emit ADF nodes. |
| Table output | `jedib0t/go-pretty/v6` | Multi-format rendering from same data model |
| Color/TTY | `fatih/color` + `mattn/go-isatty` | Auto-disables on pipe, respects `NO_COLOR` |
| XDG paths | `adrg/xdg` | Cross-platform config/data/cache directories |
| Build/release | `goreleaser` | Cross-platform builds, Homebrew tap, ldflags version embedding |

### go.mod

```
module github.com/endgameio/jira-cli

go 1.23

require (
    github.com/spf13/cobra
    github.com/hashicorp/go-retryablehttp
    github.com/BurntSushi/toml
    github.com/zalando/go-keyring
    github.com/yuin/goldmark
    github.com/jedib0t/go-pretty/v6
    github.com/fatih/color
    github.com/mattn/go-isatty
    github.com/adrg/xdg
    github.com/pkg/browser          # cross-platform browser opening for --web
    github.com/itchyny/gojq         # pure Go jq implementation for --jq flag
)
```

---

## Design Decisions & Edge Cases

### 1. Auth-free commands
`meta commands`, `meta version` (partial), `config *`, `alias *`, `--help`, `--version`, and `completion` must work without credentials. The root `PersistentPreRunE` must NOT eagerly resolve auth — it only sets global flag values (color, verbose). Auth resolution happens lazily inside `Factory.APIClient()` / `Factory.AuthResolver()`, called only when commands actually need the API. Commands that don't call the API never trigger auth.

### 2. User resolution algorithm (`--assignee`)
When a user-identifying string is provided (for `--assignee`, `assign`, etc.):
1. If it matches `@me` → call `/myself`, use that accountId
2. If it looks like an account ID (starts with a hex-like pattern, 24+ chars) → use directly
3. Otherwise → call `GET /rest/api/3/user/search?query={input}`
   - Exactly 1 result → use that accountId
   - 0 results → `CLIError(NOT_FOUND, "No user matching '...'", suggestion: "Use 'jira user search' to find users")`
   - 2+ results → `CLIError(VALIDATION_ERROR, "Ambiguous user '...'", context: {matches: [{accountId, displayName, email}, ...]}, suggestion: "Provide a more specific query or use an account ID")`

### 3. JQL composition from filter flags
When `--jql` is absent, flags compose with AND:
```
--project PROJ          → project = "PROJ"
--assignee @me          → assignee = currentUser()
--assignee "Jane"       → assignee = "accountId"  (after resolution)
--status "In Progress"  → status = "In Progress"
--type Bug              → issuetype = "Bug"
--label urgent          → labels = "urgent"
```
Multiple flags join with `AND`. Order: project, type, status, assignee, label. When `--jql` IS provided, all other filter flags are ignored (spec: explicit JQL takes precedence).

### 4. `--field key=value` custom field parsing
Split on first `=` only (values may contain `=`). Values are always sent as strings to the API. For structured values (arrays, objects), users must use `--json` input or the API's own coercion. Repeatable: `--field customfield_10001=High --field customfield_10002=3`.

### 5. Instance URL normalization
1. Strip protocol prefix (`https://`, `http://`)
2. Strip trailing slashes
3. Strip `/rest/...` path suffix if someone pastes a full API URL
4. Prepend `https://` when constructing base URL for API calls
5. Store the bare hostname (e.g., `mycompany.atlassian.net`) in config

### 6. `--dry-run` output format
- With `--json`: Structured JSON to stdout (as spec shows)
- Without `--json`: Human-readable summary to stdout:
  ```
  [DRY RUN] Would create issue:
    Project:  PROJ
    Type:     Bug
    Summary:  Login fails on Safari
    Priority: High
  Validation: OK
  ```

### 7. Error rendering ownership
Single owner: `main.go` owns all error rendering. Commands return errors (including `CLIError`). They never write errors to stderr themselves. `main.go` checks if `--json` is active, renders structured JSON or text accordingly, then calls `os.Exit()`. This prevents double-rendering.

### 8. `--web` browser opening
Use `github.com/pkg/browser` — 3-method library that handles macOS (`open`), Linux (`xdg-open`), and Windows (`start`). For `issue view --web`, construct `https://{instance}/browse/{key}` and open it. Exit 0 immediately — no API call needed beyond auth validation.

### 9. Keyring testing
`zalando/go-keyring` provides `keyring.MockInit()` for tests. Call it in test setup to redirect all keyring operations to an in-memory map. No OS keyring interaction in tests.

---

## Project Structure

```
jira-cli/
├── cmd/jira/main.go                          # Entrypoint: factory → root cmd → execute → exit code
├── internal/
│   ├── version/version.go                    # Build-time version vars (ldflags)
│   ├── errors/errors.go                      # CLIError type, error codes, exit codes, constructors
│   ├── iostreams/iostreams.go                # IOStreams: In/Out/Err, TTY detection, color control
│   ├── config/
│   │   ├── paths.go                          # XDG path resolution
│   │   ├── config.go                         # Config interface + TOML implementation
│   │   └── profile.go                        # Profile CRUD
│   ├── auth/
│   │   ├── resolver.go                       # Flags → env → profile credential chain
│   │   ├── keyring.go                        # Keyring store/retrieve/delete
│   │   ├── keyring_fallback.go               # Plaintext fallback with warning
│   │   └── credentials.go                    # Credentials type + validation (/myself)
│   ├── api/
│   │   ├── client.go                         # Client interface + httpClient implementation
│   │   ├── transport.go                      # Auth RoundTripper (Basic auth injection)
│   │   ├── retry.go                          # Retry policy (429 + Retry-After)
│   │   ├── errors.go                         # Jira error response → CLIError
│   │   ├── types.go                          # Jira API types (Issue, User, Transition, etc.)
│   │   ├── issues.go                         # Issue CRUD methods
│   │   ├── comments.go                       # Comment CRUD methods
│   │   ├── users.go                          # User search + myself
│   │   ├── schema.go                         # Fields, types, statuses, priorities, labels
│   │   └── pagination.go                     # Token-based + offset-based paginators
│   ├── output/
│   │   ├── formatter.go                      # OutputData / OutputList / OutputError / OutputDryRun
│   │   ├── table.go                          # go-pretty table utilities + styles
│   │   ├── json.go                           # JSON encoder + pagination envelope
│   │   └── errors.go                         # Structured error rendering
│   ├── adf/
│   │   ├── nodes.go                          # ADF node type definitions
│   │   ├── converter.go                      # Goldmark AST → ADF converter
│   │   └── converter_test.go                 # Table-driven tests
│   ├── factory/factory.go                    # Lazy-init Factory (IOStreams, Config, Auth, APIClient)
│   └── cmd/
│       ├── root/root.go                      # Root cmd: global flags, PersistentPreRunE, registration
│       ├── auth/{auth,login,status,logout,switch}.go
│       ├── issue/{issue,view,create,edit,move,assign,delete,list,transitions}.go
│       ├── comment/{comment,list,add,edit,delete}.go
│       ├── search/search.go
│       ├── project/{project,list,view}.go
│       ├── user/{user,search,me}.go
│       ├── schema/{schema,fields,types,statuses,priorities,labels}.go
│       ├── meta/{meta,commands,version}.go
│       ├── config_cmd/{config,set,get,list}.go
│       └── alias/{alias,set,list}.go
├── testdata/                                 # Golden files
├── .goreleaser.yaml
├── Makefile
├── go.mod
└── SPEC.md
```

Each command file follows the pattern: `NewCmdXxx(f *factory.Factory) *cobra.Command` constructor with `XxxOptions` struct and `xxxRun(ctx, opts)` function. No `init()`, no global state.

---

## Dependency Graph (acyclic)

```
main.go → cmd/root
  └── factory → iostreams, config, auth, api
        ├── config → config/paths (xdg)
        ├── auth → config, (go-keyring)
        ├── api → auth, errors, (go-retryablehttp)
        └── iostreams → (go-isatty, fatih/color)

All cmd/* → factory + api + output + errors + (adf for create/edit/comment)

output → iostreams, errors, (go-pretty)
adf → (goldmark)
errors → (nothing internal — leaf node)
```

---

## Core Abstractions

### Factory (DI hub)
```go
type Factory struct {
    IOStreams     *iostreams.IOStreams
    Config       func() (config.Config, error)    // lazy
    AuthResolver func() (*auth.Credentials, error) // lazy
    APIClient    func() (*api.Client, error)       // lazy
    Profile, FlagInstance, FlagUser, FlagToken, JQExpr string
    OutputJSON, NoColor, Verbose, DryRun, Quiet bool
}
```

### CLIError (agent-friendly errors)
```go
type CLIError struct {
    Code       ErrorCode              `json:"code"`
    Message    string                 `json:"message"`
    Context    map[string]interface{} `json:"context,omitempty"`
    Suggestion string                 `json:"suggestion,omitempty"`
    ExitCode   int                    `json:"-"`
    Err        error                  `json:"-"`
}
```
Constructors: `NewAuthError`, `NewValidationError`, `NewNotFoundError`, `NewPermissionError`, `NewRateLimitError`, `NewTransitionError`, `NewMissingFieldError`, `NewUnknownTypeError`, `NewAmbiguousUserError`, `NewNetworkError`, `NewConflictError`. Exit codes: 0=success, 1=general, 2=auth, 3=validation, 4=not-found, 5=permission, 6=rate-limited, 7=network-error, 8=conflict.

### Output Formatter
```go
type Formatter struct { ios *iostreams.IOStreams; asJSON bool }
func (f *Formatter) OutputData(data any, tableFunc func(io.Writer) error) error
func (f *Formatter) OutputList(items any, pagination *PaginationMeta, tableFunc func(io.Writer) error) error
func (f *Formatter) OutputMutation(result any, tableFunc func(io.Writer) error) error
func (f *Formatter) OutputDryRun(action string, payload any, validation string) error
// OutputError is a standalone function (not on Formatter) — used by main.go which may not have a Formatter:
func OutputError(err *CLIError, asJSON bool, w io.Writer)
```

### Auth Resolver Chain
Flags (all 3 must be set) → env vars (`JIRA_INSTANCE`, `JIRA_USER`, `JIRA_TOKEN`) → active profile (keyring for token).

### API Client
Thin `Client` interface with typed methods (`GetIssue`, `CreateIssue`, `SearchJQL`, etc.) backed by `httpClient` struct with `Do(ctx, method, path, body, out)` core method. Auth injected via `authTransport` RoundTripper. Retries via `go-retryablehttp` with custom backoff respecting `Retry-After`.

### Pagination
- **Token-based** (search): New `POST /rest/api/3/search/jql` uses `nextPageToken`. `--offset` implemented as skip-N-results (consume-and-discard). No `total` in response.
- **Offset-based** (comments, projects, labels, users): Standard `startAt`/`maxResults`/`total`.
- JSON envelope: `{"data": [...], "pagination": {"offset": 0, "limit": 50, "total": 237, "has_next_page": true}}`. `total` omitted (nil `*int`) for token-based.

### `jira meta commands`
Walks cobra command tree at runtime. Receives `rootCmd *cobra.Command` at construction. Emits only leaf commands (with `RunE`). Parses `Use` string for positional args (`<name>` = required, `[name]` = optional). Reads `MarkFlagRequired` annotations for flag required-ness. Always JSON output.

### Markdown → ADF
Goldmark parses CommonMark → AST. Custom walker emits ADF nodes. Mapping: paragraph→paragraph, heading→heading(level), emphasis(1)→em mark, emphasis(2)→strong mark, code span→code mark, fenced code→codeBlock(language), link→link mark, bullet list→bulletList, ordered list→orderedList, blockquote→blockquote, thematic break→rule.

---

## API Endpoints Map

| Command | Endpoint |
|---------|----------|
| auth login/status | `GET /rest/api/3/myself` |
| issue view | `GET /rest/api/3/issue/{key}` |
| issue create | `POST /rest/api/3/issue` |
| issue edit | `PUT /rest/api/3/issue/{key}` |
| issue delete | `DELETE /rest/api/3/issue/{key}` |
| issue assign | `PUT /rest/api/3/issue/{key}/assignee` |
| issue transitions | `GET /rest/api/3/issue/{key}/transitions` |
| issue move | `POST /rest/api/3/issue/{key}/transitions` |
| issue list / search | `POST /rest/api/3/search/jql` (new endpoint) |
| comment list | `GET /rest/api/3/issue/{key}/comment` |
| comment add | `POST /rest/api/3/issue/{key}/comment` |
| comment edit | `PUT /rest/api/3/issue/{key}/comment/{id}` |
| comment delete | `DELETE /rest/api/3/issue/{key}/comment/{id}` |
| project list | `GET /rest/api/3/project/search` |
| project view | `GET /rest/api/3/project/{key}` |
| user search | `GET /rest/api/3/user/search?query=` |
| user me | `GET /rest/api/3/myself` |
| schema fields | `GET /rest/api/3/field` or `/issue/createmeta/{proj}/issuetypes/{typeId}` |
| schema types | `GET /rest/api/3/issue/createmeta/{proj}/issuetypes` |
| schema statuses | `GET /rest/api/3/status` or `GET /rest/api/3/project/{key}/statuses` |
| schema priorities | `GET /rest/api/3/priority` |
| schema labels | `GET /rest/api/3/label` |

**Note:** Old `/rest/api/3/search` and `/rest/api/3/issue/createmeta` are deprecated. Use the new endpoints above.

---

## Implementation Phases

### Phase 0: Project Bootstrap

**Files:** `go.mod`, `Makefile`, `CLAUDE.md`, `.gitignore`

**Actions:**
- `go mod init github.com/endgameio/jira-cli`
- `go get` all 11 dependencies
- Create `Makefile` with targets: `build`, `test`, `lint`, `install`, `clean`
- Create `CLAUDE.md` with project conventions (module path, command constructor pattern, test patterns, no `init()` rule, error handling conventions)
- Create `.gitignore` for Go binaries, vendor, OS files

### Phase 1: Skeleton + Auth + Config (Foundation)

**Files:** `cmd/jira/main.go`, `internal/{version,errors,iostreams,config,auth,api,output,factory}/*`, `internal/cmd/{root,auth,config_cmd,alias}/*`

**Acceptance:**
- `jira auth login --instance x.atlassian.net --user a@b.com --token abc` validates via `/myself`, stores to keyring
- `jira auth status [--json]` shows profile info + live token check
- `jira auth logout --yes` removes credentials
- `jira auth switch staging` switches active profile
- `jira config set/get/list` works
- Credential chain: flags > env > profile
- `JIRA_INSTANCE=... JIRA_USER=... JIRA_TOKEN=... jira auth status` works without `login`
- Errors to stderr, data to stdout, correct exit codes

### Phase 2: Core Issue CRUD

**Files:** `internal/api/{types,issues,users,pagination}.go`, `internal/adf/*`, `internal/cmd/issue/*`

**Acceptance:**
- `jira issue view PROJ-123 [--json] [--fields f1,f2] [--comments] [--web]`
- `jira issue create --project PROJ --type Bug --summary "..." [--description md] [--body-file -] [--dry-run]` with Markdown→ADF
- `jira issue edit PROJ-123 --summary "new" [--add-labels x] [--dry-run]`
- `jira issue move PROJ-123 "In Progress"` — on invalid status, structured error with available transitions
- `jira issue assign PROJ-123 "Jane Doe"` — resolves user to accountId
- `jira issue delete PROJ-123 --yes`
- `jira issue list --project PROJ [--status "To Do"] [--jql "..."] [--json]` — composes JQL from flags
- `jira issue transitions PROJ-123 --json`

### Phase 3: Comments + Search

**Files:** `internal/api/comments.go`, `internal/cmd/{comment,search}/*`

**Acceptance:**
- `jira comment list/add/edit/delete` with Markdown→ADF, `--body-file -`, `--dry-run`, `--yes`
- `jira search "project = PROJ" [--json] [--limit] [--offset]`
- `jira search --mine [--status "In Progress"]` generates JQL
- `jira search` with no args and no `--mine` returns validation error

### Phase 4: Schema Introspection + Meta + Projects + Users (Agent Surface)

**Files:** `internal/api/schema.go`, `internal/cmd/{schema,meta,project,user}/*`

**Acceptance:**
- `jira meta commands` outputs JSON of all commands with args, flags, types, descriptions
- `jira meta version --json` shows version + API + instance
- `jira schema {fields,types,statuses,priorities,labels} [--project] [--type] --json`
- `jira user search "jane" --json` returns accountId, display name, email
- `jira user me --json`
- `jira project list/view --json`
- Full agent workflow executes: meta commands → schema types → schema fields → issue create --dry-run → issue create

### Phase 5: Polish

**Scope:** Robust ADF (nested lists, blockquotes, images), shell completions (`jira completion bash/zsh/fish`), alias expansion, `--verbose` HTTP logging, color status badges, golden file tests, all `--dry-run`/`--yes` enforcement verified.

### Phase 6: Build/Release Pipeline

**Files:** `.goreleaser.yaml`, `.github/workflows/{ci,release}.yml`

**Scope:** `go vet` + `golangci-lint` + `go test` on PR. Goreleaser on tag push. Homebrew tap. `go install` path. `jira --version` from ldflags.

---

## Testing Strategy

### Unit tests (every phase)
- **Command tests:** Each command gets `*_test.go` using `httptest.NewServer` for API mocking, `iostreams.Test()` for output capture, and table-driven tests for flag combinations.
- **API client tests:** `httptest` servers that return real Jira response shapes, testing error parsing, pagination, and retry behavior.
- **ADF converter tests:** Table-driven: markdown input string → expected ADF JSON. Use golden files for complex cases.
- **Config tests:** Use `t.TempDir()` for isolated config files. Test TOML round-trip.
- **Auth tests:** `keyring.MockInit()` for in-memory keyring. Test resolver chain (flags > env > profile) with `t.Setenv()`.

### Integration tests (Phase 5+)
- Build the binary, run against `httptest` server that simulates the full Jira API surface.
- Golden file tests for all output formats: `go test -update ./...` to regenerate.
- Pipe-safety test: verify no ANSI codes when stdout is not a TTY (use `iostreams.Test()` which sets `isTTY: false`).

### Key test scenarios
- Auth: all 3 credential sources, partial credentials (error), expired token (401)
- Issue create: missing required flags (exit 3), `--dry-run` output, Markdown→ADF content, custom fields
- Issue move: valid transition, invalid transition (structured error with alternatives), ambiguous status name
- User resolution: exact match, ambiguous (multiple), not found, `@me`
- Pagination: token-based with offset skip, offset-based, empty results, single page
- Rate limiting: 429 with Retry-After (verify retry happens)
- Error rendering: `--json` vs text mode for each error type

---

## Verification

After each phase, validate:
1. `go build ./...` compiles
2. `go test ./...` passes (run: `make test`)
3. `go vet ./...` clean (run: `make lint`)
4. Manual smoke test of new commands against a Jira Cloud instance
5. `jira meta commands` output includes all registered commands (Phase 4+)
6. Piped output (`jira ... | cat`) contains no ANSI codes
7. `--json` output is valid JSON parseable by `jq`
8. Errors go to stderr, data to stdout
9. Exit codes match spec (test with `echo $?`)
