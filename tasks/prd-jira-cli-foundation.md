# PRD: Jira CLI Foundation (Phases 0–2)

## Introduction

Build the foundation of a Go CLI tool (`jira`) wrapping the Jira Cloud REST API v3. This PRD covers bootstrap, authentication/configuration, and core issue CRUD — the minimum surface needed for a developer to authenticate, create/view/edit/move/delete issues, and search via JQL. The CLI must be strictly non-interactive (all input via flags), pipe-safe (structured output, correct exit codes), and produce agent-friendly structured errors for LLM self-correction.

This PRD covers Phases 0–2 from the implementation plan (note: Search was promoted from Phase 3 to Phase 2 scope; this PRD supersedes PLAN.md's phase boundaries where they differ). Phase 0 bootstraps the project. Phase 1 builds the auth/config/output foundation. Phase 2 delivers issue CRUD, user resolution, search, Markdown-to-ADF conversion, and pagination.

## Goals

- Ship a working `jira` binary that can authenticate against a Jira Cloud instance and perform full issue lifecycle operations
- Establish the internal architecture (factory, API client, error system, output formatter) that all future phases build on
- Support three credential sources (flags > env vars > stored profile) for developer and CI/CD use
- Produce structured JSON errors with context and suggestions so LLM agents can self-correct
- Convert Markdown input to Atlassian Document Format (ADF) for descriptions and comments
- Handle pagination (token-based for search, offset-based for other list endpoints)

## User Stories

---

### US-001: Project Bootstrap

**Description:** As a developer, I want the Go project initialized with all dependencies, build tooling, and conventions documented so that development can begin immediately.

**Acceptance Criteria:**
- [ ] `go.mod` created with module path `github.com/endgame-build/jira-cli`, Go 1.23, all 11 dependencies from PLAN.md (original 10 + `itchyny/gojq` for `--jq` support)
- [ ] `go.sum` generated via `go mod tidy`
- [ ] `Makefile` with targets: `build` (outputs `bin/jira`), `test` (`go test ./...`), `lint` (`go vet ./...`), `install` (`go install ./cmd/jira`), `clean` (`rm -rf bin/`)
- [ ] `.gitignore` covers: `bin/`, `*.exe`, `.DS_Store`, `vendor/`, `*.test`, `*.out`, `dist/`
- [ ] `CLAUDE.md` documents: module path, command constructor pattern (`NewCmdXxx(f *factory.Factory) *cobra.Command`), test patterns (`httptest` + `iostreams.Test()`), no `init()` rule, error handling conventions (commands return errors, `main.go` renders)
- [ ] `cmd/jira/main.go` compiles and exits 0 (stub entrypoint)
- [ ] Directory structure matches PLAN.md layout (all `internal/` package directories created)
- [ ] `go build ./...` succeeds
- [ ] `go vet ./...` clean
- [ ] Tests pass: `go test ./...`

---

### US-002: CLI Error System

**Description:** As a CLI consumer (human or agent), I want all errors to include a machine-readable code, human message, contextual data, and actionable suggestion so that I can self-correct without guessing.

**Acceptance Criteria:**
- [ ] `internal/errors/errors.go` defines `CLIError` struct with fields: `Code` (ErrorCode), `Message` (string), `Context` (map[string]interface{}), `Suggestion` (string), `ExitCode` (int), `Err` (wrapped error)
- [ ] `CLIError` implements `error` interface and supports `errors.Is`/`errors.As` unwrapping
- [ ] Error codes defined as constants: `GENERAL_ERROR`, `AUTH_ERROR`, `VALIDATION_ERROR`, `NOT_FOUND`, `PERMISSION_DENIED`, `RATE_LIMITED`, `INVALID_TRANSITION`, `MISSING_FIELD`, `UNKNOWN_TYPE`, `AMBIGUOUS_USER`, `NETWORK_ERROR`, `CONFLICT_ERROR`
- [ ] Exit codes: 0=success, 1=general, 2=auth, 3=validation, 4=not-found, 5=permission, 6=rate-limited, 7=network-error, 8=conflict
- [ ] Constructor functions: `NewAuthError`, `NewValidationError`, `NewNotFoundError`, `NewPermissionError`, `NewRateLimitError`, `NewTransitionError(currentStatus string, availableTransitions []Transition)`, `NewMissingFieldError(fieldName, fieldType string)`, `NewUnknownTypeError(given string, validTypes []string)`, `NewAmbiguousUserError(query string, matches []UserMatch)`, `NewNetworkError(err error)` (wraps connection refused, DNS failure, TLS errors, timeouts with suggestion: `"Check your network connection and that '{instance}' is reachable"`), `NewConflictError(key string)` (409 concurrent modification with suggestion: `"The issue was modified by another user. Retry your operation."`)
- [ ] JSON marshaling produces spec-compliant structure: `{"error": {"code": "...", "message": "...", "context": {...}, "suggestion": "..."}}`
- [ ] Tests: each constructor produces correct code, exit code, and JSON shape (table-driven)

---

### US-003: IOStreams and TTY Detection

**Description:** As a developer building commands, I need a centralized IO abstraction that handles stdout/stderr separation, TTY detection, and color control so that output is pipe-safe by default.

**Acceptance Criteria:**
- [ ] `internal/iostreams/iostreams.go` defines `IOStreams` struct with `In` (io.Reader), `Out` (io.Writer), `Err` (io.Writer), `IsStdoutTTY()` bool, `IsStderrTTY()` bool
- [ ] `New()` constructor detects real TTY via `mattn/go-isatty`, respects `NO_COLOR` env var
- [ ] `Test()` constructor returns IOStreams backed by `bytes.Buffer` for stdout/stderr with `isTTY: false`
- [ ] Color helper: `IOStreams.ColorEnabled()` returns false when not TTY, `--no-color` set, or `NO_COLOR` env present
- [ ] Color scheme methods using `fatih/color`: `Green()`, `Yellow()`, `Red()`, `Bold()`, `Cyan()` — all no-op when color disabled
- [ ] Pager support: `StartPager()` and `StopPager()` methods. When stdout is a TTY, pipe output through pager (`JIRA_PAGER` > `PAGER` > system default). No pager when: not a TTY, `--json` is active, or `--no-pager` flag is set. `--no-pager` is a boolean flag on IOStreams (set by the root command's `PersistentPreRunE`), NOT a global persistent flag — it is registered on specific commands that produce long output (e.g., `issue view`, `issue list`, `search`).
- [ ] Tests: TTY detection mocking, color disable via env, pipe-safety (no ANSI when not TTY)

---

### US-004: Configuration System

**Description:** As a developer, I want persistent configuration (default project, output format, color preference) stored in a TOML file at the XDG config path so that I don't repeat flags every invocation.

**Acceptance Criteria:**
- [ ] `internal/config/paths.go` resolves config dir via `adrg/xdg`: `$XDG_CONFIG_HOME/jira-cli/` (typically `~/.config/jira-cli/`)
- [ ] `internal/config/config.go` defines `Config` interface with `Get(key) string`, `Set(key, value)`, `Delete(key)`, `List() map[string]string`, `Save() error`
- [ ] TOML implementation reads/writes `config.toml` with sections: `[defaults]` (project, assignee), `[output]` (format, color), `[aliases]`
- [ ] Supported keys: `default.project`, `default.assignee`, `output.format` (text|json), `output.color` (auto|always|never)
- [ ] `internal/config/profile.go` manages named profiles: `ActiveProfile() string`, `SetActiveProfile(name)`, `ListProfiles() []string`, `DeleteProfile(name)`
- [ ] Profile metadata stored in `config.toml` under `[profiles]` section (instance, user per profile); tokens stored separately in keyring
- [ ] Config writes use file-level locking (advisory lock via `os.O_CREATE|os.O_WRONLY` + `syscall.Flock` or write-to-temp-then-rename pattern) to prevent corruption from concurrent `jira` processes
- [ ] Tests: TOML round-trip with `t.TempDir()`, profile CRUD, missing config file creates default

---

### US-005: Credential Storage (Keyring + Fallback)

**Description:** As a developer, I want my API token stored securely in the OS keyring so that credentials aren't in plaintext config files. If keyring is unavailable, fall back to file storage with a warning.

**Acceptance Criteria:**
- [ ] `internal/auth/keyring.go` wraps `zalando/go-keyring` with service name `jira-cli` and key format `{profile}-token`
- [ ] Methods: `StoreToken(profile, token) error`, `RetrieveToken(profile) (string, error)`, `DeleteToken(profile) error`
- [ ] `internal/auth/keyring_fallback.go` stores token in config dir file (`tokens.json`) when keyring fails, prints warning to stderr on every use: `"Warning: Token stored in plaintext at {path}. Install a keyring provider for secure storage."`
- [ ] Keyring failure detection: attempt keyring store, if error, fall back transparently
- [ ] Tests: `keyring.MockInit()` for in-memory keyring in all tests, no OS keyring interaction

---

### US-006: Auth Resolver Chain

**Description:** As a developer or CI pipeline, I want credentials resolved from flags first, then env vars, then stored profile — so CI can use env vars and developers use stored profiles without conflict.

**Acceptance Criteria:**
- [ ] `internal/auth/credentials.go` defines `Credentials` struct: `Instance`, `User`, `Token` (all strings)
- [ ] `internal/auth/resolver.go` implements `Resolve(flagInstance, flagUser, flagToken, profileName string, config Config) (*Credentials, error)`
- [ ] Resolution order: (1) if all three flags provided, use them; (2) if `JIRA_INSTANCE` + `JIRA_USER` + `JIRA_TOKEN` env vars all set, use them; (3) look up active profile (or named profile) from config + keyring
- [ ] Partial credentials at any level are an error: e.g., `--instance` without `--user` and `--token` → `CLIError(VALIDATION_ERROR, "Incomplete credentials: --instance, --user, and --token must all be provided together")`
- [ ] Instance URL normalization: strip `https://`, `http://`, trailing slashes, `/rest/...` suffix
- [ ] No credentials at any level (no flags, no env vars, no stored profile): `CLIError(AUTH_ERROR, "No credentials found. Run 'jira auth login' to configure authentication, or set JIRA_INSTANCE, JIRA_USER, JIRA_TOKEN environment variables.")`, exit 2
- [ ] Tests: all 3 sources independently, partial credentials error, no credentials at all error, flag override of env, env override of profile, URL normalization edge cases (using `t.Setenv()` for env vars)

---

### US-007: API Client Core

**Description:** As a developer building commands, I need a typed HTTP client that handles authentication injection, retry with backoff (respecting `Retry-After`), and Jira error response parsing.

**Acceptance Criteria:**
- [ ] `internal/api/client.go` defines `Client` struct with `Do(ctx context.Context, method, path string, body interface{}, out interface{}) error` core method
- [ ] Constructor: `NewClient(credentials *auth.Credentials, opts ...ClientOption) *Client`
- [ ] `internal/api/transport.go` implements `http.RoundTripper` that injects `Authorization: Basic base64(user:token)` header and `Content-Type: application/json`
- [ ] `internal/api/retry.go` wraps `hashicorp/go-retryablehttp`: retries on 429 and 5xx only, respects `Retry-After` header for 429, max 3 retries, exponential backoff for 5xx. **Never retry**: 401 (abort immediately with `AUTH_ERROR` + re-login suggestion), request timeouts (already waited 30s), 4xx other than 429
- [ ] `internal/api/errors.go` parses Jira `ErrorCollection` response (`errorMessages` array + `errors` map) into appropriate `CLIError` with context
- [ ] Success status handling: `Client.Do()` accepts 200, 201, and 204 as success. On 204 (No Content), skip response body decoding (return nil for `out` parameter). On 201, decode normally.
- [ ] HTTP error mapping: 400→VALIDATION_ERROR (exit 3), 401→AUTH_ERROR (exit 2, suggestion: `"Run 'jira auth login' to refresh credentials"`), 403→PERMISSION_DENIED (exit 5), 404→NOT_FOUND (exit 4), 409→CONFLICT_ERROR (exit 8), 429→RATE_LIMITED (exit 6, context includes retry-after seconds)
- [ ] Network error handling: connection refused, DNS resolution failure (`no such host`), TLS handshake errors, and request timeouts all produce `CLIError(NETWORK_ERROR, "...")` with exit 7 and actionable suggestion including the instance hostname
- [ ] Request timeout: 30-second default per request; configurable via `ClientOption`
- [ ] Base URL construction: `https://{instance}/rest/api/3/{path}`
- [ ] All response bodies closed properly (no leaks)
- [ ] Tests: `httptest.NewServer` for each status code (including 201 and 204), retry behavior on 429 with Retry-After, no-retry on 401 and timeouts, auth header injection, error parsing from real Jira error shapes, network error wrapping (connection refused, timeout)

---

### US-008: API Types (Jira Data Model)

**Description:** As a developer, I need Go structs matching the Jira v3 API response shapes so that JSON unmarshaling is type-safe and fields are discoverable.

**Acceptance Criteria:**
- [ ] `internal/api/types.go` defines structs grounded against the Swagger spec:
  - `User` — `AccountID`, `DisplayName`, `EmailAddress` (`*string` — nullable due to Jira privacy settings), `Active`, `AvatarUrls`, `Self`
  - `Issue` — `ID`, `Key`, `Self`, `Fields` (typed sub-struct, not raw map)
  - `IssueFields` — `Summary`, `Description` (json.RawMessage for ADF), `Status`, `IssueType`, `Priority`, `Assignee` (*User), `Reporter` (*User), `Labels`, `Project`, `Created`, `Updated`, `Parent` (*IssueParent), `Comment` (*CommentPage), `IssueLinks` ([]IssueLink), `Subtasks` ([]LinkedIssue)
  - `Status` — `ID`, `Name`, `StatusCategory`
  - `StatusCategory` — `ID`, `Key`, `Name`, `ColorName`
  - `IssueType` — `ID`, `Name`, `Description`, `Subtask`
  - `Priority` — `ID`, `Name`, `Description`, `IconURL`
  - `Project` — `ID`, `Key`, `Name`
  - `IssueParent` — `ID`, `Key`
  - `IssueLink` — `ID`, `Type` (name, inward, outward), `InwardIssue` (*LinkedIssue), `OutwardIssue` (*LinkedIssue)
  - `LinkedIssue` — `ID`, `Key`, `Self`, `Fields` (*LinkedIssueFields). See `LinkedIssueFields` below for nested fields.
  - `Transition` — `ID`, `Name`, `To` (*Status), `HasScreen`, `IsGlobal`, `IsAvailable`
  - `Comment` — `ID`, `Self`, `Body` (json.RawMessage for ADF), `Author` (*User), `Created`, `Updated`
  - `CommentPage` — `Comments` ([]Comment), `StartAt`, `MaxResults`, `Total`
  - `CreatedIssue` — `ID`, `Key`, `Self`
  - `SearchResults` — `Issues`, `NextPageToken`, `IsLast` (note: no `total`, `startAt`, or `maxResults` — this is the new `SearchAndReconcileResults` schema, not the legacy `SearchResults`)
  - `CreateMetaIssueTypes` — `IssueTypes` ([]IssueTypeCreateMeta, preferred field), `StartAt`, `MaxResults`, `Total` (note: response also contains legacy `.createMetaIssueType` array — read `.issueTypes` as primary)
  - `ErrorCollection` — `ErrorMessages` ([]string), `Errors` (map[string]string), `Status`
  - `LinkedIssueFields` — `Summary` (string), `Status` (*Status), `Priority` (*Priority), `IssueType` (*IssueType). Note: Jira API returns subtask and linked issue objects with a nested `fields` wrapper: `{"id": "...", "key": "...", "fields": {"summary": "...", ...}}`. The `LinkedIssue` struct above has `ID`, `Key`, and a `Fields` sub-struct of type `LinkedIssueFields`.
  - `IssueTypeCreateMeta` — `ID` (string), `Name`, `Description`, `Subtask` (bool), `Scope` (project/global). Returned by `GET /issue/createmeta/{project}/issuetypes` in the `.issueTypes` array.
  - `DoTransitionInput` — `TransitionID` (string, required), `Resolution` (*string, optional — sent as `fields.resolution.name`), `Comment` (*adf.Document, optional — sent as `update.comment[].add.body`)
  - `PaginationOptions` — `Limit` (int), `Offset` (int) — used by offset-based and raw-array paginated endpoints (user search, comments, projects, labels)
  - `GetIssueOptions` — `Fields` ([]string), `Expand` ([]string), `Comments` (bool)
  - `CreateIssueInput` — `Project` (string), `Type` (string), `Summary` (string), `Description` (*adf.Document), `Assignee` (*string, accountId), `Priority` (*string), `Labels` ([]string), `Parent` (*string, issue key), `CustomFields` (map[string]string)
  - `EditIssueInput` — `Fields` (map[string]interface{} for direct-set fields), `Update` (map[string][]UpdateOp for add/remove operations). `UpdateOp` has `Add`/`Remove` *string.
  - `SearchOptions` — `Fields` ([]string), `Limit` (int), `PageToken` (*string)
- [ ] All structs have `json` tags matching the Swagger spec field names exactly
- [ ] `IssueFields` uses typed sub-structs (not `map[string]interface{}`) for known fields, plus `CustomFields map[string]json.RawMessage` for arbitrary fields
- [ ] Tests: JSON unmarshal round-trip against representative Jira API response payloads (golden data from Swagger examples)

---

### US-009: Output Formatter

**Description:** As a CLI consumer, I want consistent output: human-readable tables by default, structured JSON with `--json`, errors always to stderr — so that output is both human-friendly and machine-parseable.

**Acceptance Criteria:**
- [ ] `internal/output/formatter.go` defines `Formatter` struct with `IOStreams` and `asJSON bool`
- [ ] `OutputData(data any, tableFunc func(io.Writer) error) error` — JSON or table for single objects
- [ ] `OutputList(items any, pagination *PaginationMeta, tableFunc func(io.Writer) error) error` — JSON with pagination envelope or table for lists
- [ ] `OutputMutation(result any, tableFunc func(io.Writer) error) error` — JSON or human text for mutating command success (create, edit, move, assign, delete, login, logout, switch)
- [ ] `OutputDryRun(action string, payload any, validation string) error` — JSON dry-run envelope or human-readable summary
- [ ] `internal/output/jq.go` — `ApplyJQ(data any, expr string) (string, error)` using `itchyny/gojq` (pure Go). When `--jq` is set, JSON output is passed through the jq expression before writing to stdout. Output is raw text (not JSON-wrapped) — matching `gh --jq` behavior.
- [ ] `internal/output/json.go` — JSON encoder with three envelope shapes:
  - **Single object** (`OutputData`): bare object `{...}` — no wrapping (e.g., `issue view --json` returns the issue directly). Agents parse the root object.
  - **List** (`OutputList`): `{"data": [...], "pagination": {"offset": N, "limit": N, "total": N, "has_next_page": bool}}`; `total` omitted (pointer nil) for token-based pagination
  - **Mutation** (`OutputMutation`): `{"ok": true, ...fields}` — e.g., `{"ok": true, "key": "PROJ-124", "url": "..."}` for create, `{"ok": true, "key": "PROJ-123", "action": "moved", "to": "In Progress"}` for move. The `ok` field lets agents distinguish success from error without checking exit code.
- [ ] `internal/output/table.go` — `go-pretty` table utilities: `NewTable(w io.Writer) Table`, default style (no borders, aligned columns), color support via IOStreams
- [ ] `internal/output/errors.go` — `OutputError(err *CLIError, asJSON bool, w io.Writer)` renders to stderr in JSON or text format
- [ ] Data always to stdout, errors/warnings always to stderr
- [ ] Tests: JSON output is valid JSON (parse with `json.Unmarshal`), table output has no ANSI when `isTTY: false`, pagination envelope shape, `--jq` expression filtering (simple field extraction, array length, nested access)

---

### US-010: Factory (Dependency Injection Hub)

**Description:** As a developer building commands, I need a single factory object that lazily initializes IOStreams, Config, Auth, and APIClient so that commands are testable and auth-free commands never trigger credential resolution.

**Acceptance Criteria:**
- [ ] `internal/factory/factory.go` defines `Factory` struct with lazy accessors:
  - `IOStreams` — `*iostreams.IOStreams` (eager, set at construction)
  - `Config() (config.Config, error)` — lazy, cached after first call
  - `AuthResolver() (*auth.Credentials, error)` — lazy, calls auth.Resolve with current flags/env/profile
  - `APIClient() (*api.Client, error)` — lazy, calls AuthResolver then constructs Client
- [ ] Global flag storage on Factory: `Profile`, `FlagInstance`, `FlagUser`, `FlagToken`, `OutputJSON`, `NoColor`, `Verbose`, `DryRun`, `Quiet`, `JQExpr`
- [ ] Auth-free commands (`meta`, `config`, `alias`, `--help`, `--version`) never call `AuthResolver()` or `APIClient()` — verified by test
- [ ] `NewTestFactory(ios *iostreams.IOStreams, cfg config.Config, client *api.Client) *Factory` for tests — pre-wired, no lazy resolution
- [ ] Tests: lazy init verified (APIClient not called until accessed), test factory wiring

---

### US-011: Root Command and Global Flags

**Description:** As a CLI user, I want global flags (`--profile`, `--json`, `--no-color`, `--verbose`, `--dry-run`, `--instance`, `--user`, `--token`) available on every command, with `PersistentPreRunE` that only configures output settings (not auth).

**Acceptance Criteria:**
- [ ] `internal/cmd/root/root.go` defines `NewCmdRoot(f *factory.Factory) *cobra.Command`
- [ ] Persistent flags registered: `--profile` (string), `--json` (bool), `--no-color` (bool), `--verbose` (bool), `--dry-run` (bool), `--quiet` / `-q` (bool), `--jq` (string), `--instance` (string), `--user` (string), `--token` (string)
- [ ] `PersistentPreRunE` reads flag values into Factory fields (`OutputJSON`, `NoColor`, `Verbose`, `DryRun`, `Quiet`, `JQExpr`, `Profile`, `FlagInstance`, `FlagUser`, `FlagToken`), configures IOStreams color — does NOT resolve auth
- [ ] JSON output resolution: `--json` flag (explicit) > `output.format` config value > default (`text`). When `output.format = json` in config, all commands default to JSON output without needing `--json` flag. There is no `--no-json` flag — to override config-level JSON back to text for a single invocation, use `--text` flag (bool, mutually exclusive with `--json`)
- [ ] Persistent flags include `--text` (bool) in addition to `--json` (bool); if both set: `CLIError(VALIDATION_ERROR, "Cannot use --json and --text together")`
- [ ] `--jq` implies `--json` — if `--jq` is set without `--json`, auto-enable JSON mode. `--jq` with `--text` is a validation error: `CLIError(VALIDATION_ERROR, "Cannot use --jq and --text together")`
- [ ] `--quiet` / `-q` — suppresses non-essential stdout on mutating commands (success message omitted, exit 0 is sufficient). Incompatible with `--json` and `--jq`: `CLIError(VALIDATION_ERROR, "Cannot use --quiet with --json or --jq")`. `--quiet` + `--dry-run`: `--dry-run` output is NOT suppressed (dry-run is the whole point of the invocation; `--quiet` is silently ignored when `--dry-run` is active)
- [ ] `--verbose` — registered as persistent flag but is a no-op in Phases 0–2 (flag value stored in Factory but no consumer). HTTP debug logging deferred to Phase 5. Registering now avoids breaking flag changes later.
- [ ] `--quiet` on read commands (e.g., `issue view`, `issue list`, `search`, `transitions`): silently ignored (no effect, no error). Read commands always produce output.
- [ ] `--dry-run` on read commands: silently ignored (no effect, no error). Read commands are inherently non-mutating.
- [ ] Enable Cobra's `SuggestionsMinimumDistance = 2` on the root command for typo suggestions (e.g., `jira isue` → `Did you mean "issue"?`)
- [ ] Subcommand groups registered: `auth`, `issue`, `search`, `config`, `alias`. Future groups (`comment`, `project`, `user`, `schema`, `meta`) are NOT registered in Phases 0–2 — they will be added in their respective phases to avoid empty command groups confusing agents and `--help` output
- [ ] `--help` and `--version` work without credentials
- [ ] Tests: global flags propagate to Factory, `--help` exits 0, `--json` overrides config, `--text` overrides config, `--json` + `--text` conflict error, `--jq` implies `--json`, `--jq` + `--text` conflict, `--quiet` + `--json` conflict, `--quiet` + `--dry-run` outputs dry-run (not suppressed), `--verbose` flag registered (no-op), typo suggestion output

---

### US-012: Main Entrypoint and Error Rendering

**Description:** As a CLI consumer, I want `main.go` to be the single owner of error rendering and exit codes so that errors are never double-printed and exit codes are always correct.

**Acceptance Criteria:**
- [ ] `cmd/jira/main.go` constructs Factory, builds root command, calls `Execute()`
- [ ] On error: checks if `CLIError` (via `errors.As`), renders via `output.OutputError` to stderr, exits with `CLIError.ExitCode`
- [ ] On non-CLIError: wraps in generic `CLIError(GENERAL_ERROR, err.Error())`, exits 1
- [ ] No command ever writes errors to stderr directly — they return errors
- [ ] Version info injected via ldflags: `-X version.Version=... -X version.Commit=... -X version.Date=...`
- [ ] Tests: build binary, run with bad flags → exit 3, run with `--version` → exit 0

---

### US-013: Auth Login Command

**Description:** As a developer, I want `jira auth login --instance x.atlassian.net --user me@co.com --token abc123` to validate credentials against the Jira API and store them securely, so I can authenticate once and run commands without repeating credentials.

**Acceptance Criteria:**
- [ ] `jira auth login` requires `--instance`, `--user`, `--token` (all three)
- [ ] Validates credentials by calling `GET /rest/api/3/myself` — on success, prints `"Logged in as {displayName} ({email}) on {instance}"` to stdout. If `emailAddress` is null (Jira privacy settings), print `"Logged in as {displayName} on {instance}"` (omit email gracefully)
- [ ] `--json` output: `{"ok": true, "profile": "default", "instance": "x.atlassian.net", "email": "me@co.com", "display_name": "Jane Doe"}` — `email` is `null` (not omitted) when Jira privacy settings mask it
- [ ] On 401 from `/myself`: `CLIError(AUTH_ERROR, "Authentication failed: invalid credentials")`, exit 2
- [ ] Instance URL normalized before storage (strip protocol, trailing slash, `/rest/...`)
- [ ] Credentials stored: instance + user in config TOML, token in keyring (or fallback)
- [ ] `--profile <name>` stores under named profile (default: `"default"`)
- [ ] If profile already exists, overwrites silently (re-login updates credentials)
- [ ] Tests: successful login with mock `/myself`, failed login (401), URL normalization, profile storage, null email handling

---

### US-014: Auth Status Command

**Description:** As a developer, I want `jira auth status` to show my current authentication state with a live token validity check, so I can verify my setup is working.

**Acceptance Criteria:**
- [ ] Displays: profile name, instance URL, user email (or `"(email hidden)"` if null due to privacy settings), token validity (live check via `GET /rest/api/3/myself`)
- [ ] Token valid: shows `"Token: valid"` with green indicator (when TTY)
- [ ] Token invalid (401): shows `"Token: invalid"` with red indicator, still exits 0 (status is informational)
- [ ] No stored credentials: `CLIError(AUTH_ERROR, "No credentials found. Run 'jira auth login' first.")`, exit 2
- [ ] `--json` output: `{"profile": "...", "instance": "...", "user": "...", "token_valid": true/false}`
- [ ] Tests: valid token, invalid token, no credentials, JSON output shape

---

### US-015: Auth Logout Command

**Description:** As a developer, I want `jira auth logout --yes` to remove my stored credentials so I can clean up when switching accounts or machines.

**Acceptance Criteria:**
- [ ] Requires `--yes` / `-y` flag (without it: `CLIError(VALIDATION_ERROR, "Use --yes to confirm logout")`, exit 3)
- [ ] Removes token from keyring and profile from config
- [ ] `--profile <name>` targets a specific profile (default: active profile)
- [ ] If deleting active profile, clears active profile setting
- [ ] Success: `"Logged out from profile '{name}'"` to stdout
- [ ] `--json` output: `{"ok": true, "profile": "default", "action": "logout"}`
- [ ] Profile not found: `CLIError(NOT_FOUND, "Profile '{name}' not found")`, exit 4
- [ ] Tests: successful logout, missing --yes, unknown profile, JSON output shape

---

### US-016: Auth Switch Command

**Description:** As a developer with multiple Jira instances, I want `jira auth switch <profile>` to change my active profile so I can quickly switch between work and personal accounts.

**Acceptance Criteria:**
- [ ] Positional arg: profile name (required)
- [ ] Sets active profile in config
- [ ] Success: `"Switched to profile '{name}' ({instance})"` to stdout
- [ ] `--json` output: `{"ok": true, "profile": "staging", "instance": "staging.atlassian.net"}`
- [ ] Profile not found: `CLIError(NOT_FOUND, "Profile '{name}' not found. Available: default, staging")`, exit 4
- [ ] Tests: successful switch, unknown profile with available list, JSON output shape

---

### US-017: Config Set/Get/List Commands

**Description:** As a developer, I want `jira config set/get/list` to manage persistent defaults so I don't repeat `--project PROJ` on every command.

**Acceptance Criteria:**
- [ ] `jira config set <key> <value>` — sets config value; valid keys: `default.project`, `default.assignee`, `output.format` (text|json), `output.color` (auto|always|never). `--json` output: `{"ok": true, "key": "default.project", "value": "PROJ"}`
- [ ] Invalid key: `CLIError(VALIDATION_ERROR, "Unknown config key '...'", suggestion: "Valid keys: ...")`, exit 3
- [ ] Invalid value for `output.format`: `CLIError(VALIDATION_ERROR, "Invalid value '...' for output.format", context: {allowed: ["text","json"]})`, exit 3
- [ ] `jira config get <key>` — prints value to stdout; unset key prints empty string
- [ ] `jira config list` — prints all config as `key = value` table; `--json` outputs JSON object
- [ ] These commands work WITHOUT authentication (no API calls)
- [ ] Tests: set/get round-trip, invalid key, invalid value, list format, no auth required

---

### US-018: Alias Set/List Commands

**Description:** As a developer, I want `jira alias set mine "search --mine"` to define shortcuts for frequent command patterns.

**Acceptance Criteria:**
- [ ] `jira alias set <name> <command>` — stores alias in config under `[aliases]` section. `--json` output: `{"ok": true, "name": "mine", "command": "search --mine"}`
- [ ] `jira alias list` — displays aliases as table; `--json` outputs JSON
- [ ] Alias name validation: alphanumeric + hyphens only, cannot shadow existing command names
- [ ] Shadowing error: `CLIError(VALIDATION_ERROR, "Cannot alias '{name}': conflicts with built-in command")`, exit 3
- [ ] These commands work WITHOUT authentication
- [ ] Tests: set/list round-trip, name validation, shadow detection

---

### US-019: Markdown to ADF Converter

**Description:** As a CLI user, I want to write issue descriptions and comments in Markdown and have them automatically converted to Atlassian Document Format (ADF) so I don't have to learn ADF syntax.

**Acceptance Criteria:**
- [ ] `internal/adf/nodes.go` defines ADF node types: `Document` (type:"doc", version:1), `Paragraph`, `Heading` (level 1-6), `BulletList`, `OrderedList`, `ListItem`, `CodeBlock` (language attr), `Blockquote`, `Rule`, `Text` (with marks), `HardBreak`
- [ ] Mark types: `strong`, `em`, `code`, `link` (href attr), `strike`
- [ ] `internal/adf/converter.go` uses `yuin/goldmark` to parse CommonMark → AST, then walks AST emitting ADF nodes
- [ ] Mapping: paragraph→paragraph, heading→heading(level), `*italic*`→em mark, `**bold**`→strong mark, `` `code` ``→code mark, fenced code block→codeBlock(language), `[text](url)`→link mark on text, bullet list→bulletList, ordered list→orderedList, blockquote→blockquote, `---`→rule, `~~strike~~`→strike mark
- [ ] Plain text (no Markdown) passes through as single paragraph with text node
- [ ] Nested structures: bold inside italic, links inside lists, code inside bold
- [ ] `Convert(markdown string) (*Document, error)` — public entry point
- [ ] Tests: table-driven with markdown input → expected ADF JSON output; cover each node type, nested marks, plain text passthrough, empty input

---

### US-020: Pagination Helpers

**Description:** As a developer building list/search commands, I need pagination utilities that handle both Jira's token-based search pagination and classic offset-based pagination, producing a consistent output envelope.

**Acceptance Criteria:**
- [ ] `internal/api/pagination.go` defines `PaginationMeta` struct: `Offset int`, `Limit int`, `Total *int` (pointer — nil for token-based), `HasNextPage bool`
- [ ] Token-based paginator (for `POST /rest/api/3/search/jql`): accepts `limit` and `offset` flags; `offset` implemented as skip-N-results (consume and discard); iterates using `nextPageToken`; stops when `isLast` or enough results collected
- [ ] Offset-based paginator (for comments, projects, labels): passes `startAt` and `maxResults` query params; reads `total` from response when available
- [ ] Raw-array paginator variant (for user search): some endpoints return a plain `[]T` array with no pagination envelope — no `total`, `startAt`, or `maxResults` in the response. For these, `has_next_page` is inferred: if `len(results) == maxResults`, assume more pages exist. `total` is nil in `PaginationMeta`.
- [ ] Both return `([]T, *PaginationMeta, error)` pattern
- [ ] JSON envelope via formatter: `{"data": [...], "pagination": {"offset": 0, "limit": 50, "total": 237, "has_next_page": true}}`
- [ ] **Semantic note for agents:** `pagination.offset` always reflects the *requested* `--offset` value (the number of results to skip), not a server cursor. For token-based search, `total` is omitted (null/absent) because the API doesn't provide it. For offset-based endpoints, `total` is the server-reported total (also null for raw-array endpoints like user search). Agents should use `has_next_page` to determine whether to paginate further — not `total`.
- [ ] **Performance note:** For token-based search, `--offset` is implemented as consume-and-discard — requesting `--offset 500 --limit 50` actually fetches 550 results from the API. For large offsets, agents should prefer iterating with `nextPageToken` from `--json` pagination metadata instead.
- [ ] Tests: token-based with mock multi-page responses, offset-based, raw-array pagination (inferred `has_next_page`), offset skip behavior, single page, empty results, verify `total` absent on token-based and raw-array

---

### US-021: Issue View Command

**Description:** As a developer, I want `jira issue view PROJ-123` to display a single issue with key fields so I can quickly check status without opening a browser.

**Acceptance Criteria:**
- [ ] Calls `GET /rest/api/3/issue/{key}` with `expand=transitions`
- [ ] Default table output shows: key, summary, status (with color badge by category: green=Done, yellow=In Progress, blue-gray=To Do), assignee (display name or "Unassigned"), reporter, priority, type, labels (comma-separated), created, updated
- [ ] Linked issues section: if `issuelinks` is non-empty, show `"Links:"` section with each link as `"blocks PROJ-456 (In Progress)"` / `"blocked by PROJ-789 (Done)"` using the link type's inward/outward description
- [ ] Subtasks section: if `subtasks` is non-empty, show `"Subtasks:"` section with each as `"  PROJ-125  Fix login CSS  [Done]"`
- [ ] Description shown truncated to first 5 lines with `... (truncated)` indicator
- [ ] `--fields f1,f2` — show only specified fields
- [ ] `--comments` — appends comment list (author, date, body truncated to 3 lines)
- [ ] `--json` — full issue JSON as bare object (no envelope wrapping) with all fields including links and subtasks
- [ ] `--web` — opens `https://{instance}/browse/{key}` in default browser; reads instance from stored config/profile only (no API call, no credential validation). Works offline with cached config. Only fails if no profile exists: `CLIError(AUTH_ERROR, "No profile configured. Run 'jira auth login' first.")`. `--web` + `--json`: outputs `{"ok": true, "url": "https://..."}` AND opens the browser (both actions). `--web` + `--quiet`: opens browser silently (no stdout). `--web` + `--fields` / `--comments`: `--web` takes precedence (no API call made, fields/comments flags silently ignored)
- [ ] Issue not found: `CLIError(NOT_FOUND, "Issue 'PROJ-999' not found")`, exit 4
- [ ] Tests: successful view with mock response, field filtering, `--comments`, linked issues display, subtasks display, `--web` (mock browser open, no API call made), `--web` + `--json` output shape, `--web` + `--quiet` (no stdout), `--web` + `--fields` (flags ignored), 404 error

---

### US-022: Issue Create Command

**Description:** As a developer, I want `jira issue create --project PROJ --type Bug --summary "Login fails"` to create an issue and return the key, so I can file bugs from the terminal.

**Acceptance Criteria:**
- [ ] Required flags: `--project` (string, falls back to `default.project` from config if omitted), `--type` (string), `--summary` (string)
- [ ] Optional flags: `--description` (string, Markdown→ADF), `--assignee` (user, resolved via US-024; falls back to `default.assignee` from config if omitted — `@me` and display names both valid as config values), `--priority` (string), `--labels` (comma-separated), `--parent` (issue key for subtasks), `--field` (key=value, repeatable), `--body-file` (path, `-` for stdin)
- [ ] `--body-file` reads description from file; `-` reads from stdin; `--body-file` overrides `--description` if both given
- [ ] `--body-file` error paths: (a) file not found or unreadable → `CLIError(VALIDATION_ERROR, "Cannot read file '{path}': {os error}")`, exit 3; (b) `--body-file -` when stdin is a TTY (not piped) → immediate `CLIError(VALIDATION_ERROR, "--body-file - requires piped input (e.g., echo '...' | jira issue create --body-file -)")`, exit 3; (c) stdin reads capped at 10MB → `CLIError(VALIDATION_ERROR, "Input exceeds 10MB limit")`
- [ ] Calls `POST /rest/api/3/issue` with properly structured payload: `project` as `{"key":"PROJ"}`, `issuetype` as `{"name":"Bug"}`, `description` as ADF document, `assignee` as `{"accountId":"..."}`, `labels` as `["l1","l2"]`, `priority` as `{"name":"High"}`
- [ ] `--field customfield_10001=High` sets arbitrary fields: split on first `=`, value always sent as JSON string to the API (no client-side type coercion — Jira coerces strings to the target field type server-side for most field types; revisit when `jira schema fields` is available in Phase 4)
- [ ] Success output: `"Created PROJ-124: https://{instance}/browse/PROJ-124"` to stdout
- [ ] `--json` output: `{"ok": true, "key": "PROJ-124", "id": "10001", "url": "https://..."}`
- [ ] Description size limit: `--description` flag content is capped at 10MB (same limit as `--body-file`), enforced before ADF conversion
- [ ] `--dry-run`: makes real API calls for validation only — calls `GET /rest/api/3/issue/createmeta/{project}/issuetypes` to verify project exists and issue type is valid (read from `.issueTypes` array in response, not legacy `.createMetaIssueType`), resolves `--assignee` via user search — then outputs structured preview of the payload that *would* be sent, without calling the create API. JSON mode: `{"dry_run": true, "payload": {...}, "validation": "passed (validated against live API)"}`
- [ ] **Idempotency note (for agents):** `issue create` is NOT idempotent — retrying will create duplicate issues. Agents should use `--dry-run` first, then create, and check the returned key to confirm success.
- [ ] Missing required flag: `CLIError(VALIDATION_ERROR, "Required flag --project not set")`, exit 3. More specifically: `--project` flag > `default.project` config > `CLIError(VALIDATION_ERROR, "Required flag --project not set and no default.project configured. Set with: jira config set default.project <KEY>")`, exit 3
- [ ] API 400 error (bad field): structured error with Jira's field-level error messages
- [ ] Tests: successful create, dry-run output, body-file from stdin, custom fields, missing flags, API error parsing

---

### US-023: Issue Edit Command

**Description:** As a developer, I want `jira issue edit PROJ-123 --summary "New title" --add-labels bugfix` to update specific fields on an existing issue.

**Acceptance Criteria:**
- [ ] Positional arg: issue key (required)
- [ ] At least one field flag required, otherwise: `CLIError(VALIDATION_ERROR, "At least one field flag required")`
- [ ] Field flags: `--summary`, `--description` (Markdown→ADF), `--assignee` (resolved), `--priority`, `--labels` (replaces all), `--add-labels` (append), `--remove-labels` (remove specific), `--field` (repeatable), `--body-file`
- [ ] `--labels` uses `fields` approach (direct set); `--add-labels`/`--remove-labels` use `update` approach with `{"add":"x"}` / `{"remove":"x"}` operations
- [ ] Calls `PUT /rest/api/3/issue/{key}` — `fields` and `update` must not overlap
- [ ] `--assignee ""` unassigns (sends `{"assignee": null}`)
- [ ] `--description ""` clears the description (sends empty ADF document). Same for `--summary ""`: `CLIError(VALIDATION_ERROR, "Summary cannot be empty")`, exit 3 (Jira requires a non-empty summary)
- [ ] `--field` key collision with named flags: if `--field summary=...` is used alongside `--summary`, the named flag takes precedence and a warning is printed to stderr: `"Warning: --field 'summary' ignored; use --summary flag instead"`. Same for other named fields (assignee, priority, labels, description).
- [ ] Success output: `"Updated PROJ-123"` to stdout
- [ ] `--dry-run`: fetches current issue via `GET /rest/api/3/issue/{key}`, computes full before/after diff for changed fields, outputs diff-style preview (field: old → new); JSON mode outputs `{"dry_run": true, "changes": [{"field": "summary", "from": "old", "to": "new"}, ...], "validation": "passed (validated against live API)"}`
- [ ] `--json` on success: `{"ok": true, "key": "PROJ-123", "updated_fields": ["summary", "labels"]}`
- [ ] `--labels` is mutually exclusive with `--add-labels` and `--remove-labels`: if `--labels` is combined with either, `CLIError(VALIDATION_ERROR, "Cannot use --labels with --add-labels or --remove-labels. Use --labels to replace all labels, or --add-labels / --remove-labels to modify.")`, exit 3
- [ ] Issue not found: `CLIError(NOT_FOUND, "Issue 'PROJ-999' not found")`, exit 4
- [ ] No permission: `CLIError(PERMISSION_DENIED, "No permission to edit PROJ-123")`, exit 5
- [ ] Conflict (409): `CLIError(CONFLICT_ERROR, "Issue was modified by another user")`, exit 8
- [ ] **Idempotency note (for agents):** `issue edit` is idempotent — setting the same values again is safe and produces no error.
- [ ] Tests: single field edit, multiple fields, add/remove labels, labels conflict with add/remove, unassign, dry-run with diff, no flags error, 404, 403, 409

---

### US-024: User Resolution

**Description:** As a developer using `--assignee "Jane"`, I want the CLI to automatically resolve human-readable names to Jira account IDs so I don't have to look up IDs manually.

**Acceptance Criteria:**
- [ ] Implemented as shared utility in `internal/api/users.go`: `ResolveUser(ctx, client, input string) (string, error)` returns accountId
- [ ] `@me` → calls `GET /rest/api/3/myself`, returns that accountId
- [ ] Input looks like an account ID (24+ hex-like chars) → use directly (no API call)
- [ ] Otherwise → `GET /rest/api/3/user/search?query={input}`
  - Exactly 1 result → return that accountId
  - 0 results → `CLIError(NOT_FOUND, "No user matching 'xyz'", suggestion: "Use 'jira user search' to find users")`, exit 4
  - 2+ results → `CLIError(AMBIGUOUS_USER, "Ambiguous user 'Jane'", context: {matches: [{accountId, displayName, email}, ...]}, suggestion: "Provide a more specific query or use an account ID")`, exit 3
- [ ] Tests: @me resolution, direct account ID passthrough, single match, no match, ambiguous match

---

### US-025: Issue Move (Transition) Command

**Description:** As a developer, I want `jira issue move PROJ-123 "In Progress"` to transition an issue to a new workflow status, with structured error guidance when the transition isn't available.

**Acceptance Criteria:**
- [ ] Positional args: issue key (required), target status name (required)
- [ ] Fetches current issue status via `GET /rest/api/3/issue/{key}` (needed for the `"from"` field in `--json` output), then available transitions via `GET /rest/api/3/issue/{key}/transitions`
- [ ] Matching strategy (ordered): (1) case-insensitive exact match on `transition.to.name`, (2) case-insensitive substring match on `transition.to.name` — if exactly 1 transition target contains the input as a substring, use it (e.g., `"progress"` matches `"In Progress"`); if 2+ substring matches, treat as ambiguous
- [ ] Optional flags: `--resolution` (string, e.g., `"Fixed"`, `"Won't Do"` — sent in `fields.resolution.name`), `--comment` (string, Markdown→ADF — sent in `update.comment[].add`). These allow setting resolution and adding a comment atomically with the transition (common for "Done" workflows).
- [ ] On match: calls `POST /rest/api/3/issue/{key}/transitions` with `{"transition": {"id": "..."}, "fields": {"resolution": {"name": "..."}}, "update": {"comment": [{"add": {"body": ADF}}]}}` (fields/update sections omitted when flags not provided)
- [ ] Success: `"Moved PROJ-123 to 'In Progress'"` to stdout
- [ ] `--json` output: `{"ok": true, "key": "PROJ-123", "action": "moved", "from": "Open", "to": "In Progress", "transition": "Start Progress"}` (includes `"resolution": "Fixed"` when `--resolution` provided)
- [ ] No matching transition (0 matches): `CLIError(INVALID_TRANSITION, "Status 'QA' is not reachable from current status 'Open'", context: {current_status: "Open", available_transitions: [{name: "Start Progress", target: "In Progress"}, ...]}, suggestion: "Available targets: In Progress, Done. Use 'jira issue transitions PROJ-123' for full list.")`, exit 3
- [ ] Ambiguous substring match (2+ matches): `CLIError(INVALID_TRANSITION, "Ambiguous status 'In': matches 'In Progress', 'In Review'", context: {matches: ["In Progress", "In Review"]}, suggestion: "Provide the full status name to disambiguate")`, exit 3
- [ ] Multiple transitions to same target status (rare): use first match
- [ ] Issue not found: `CLIError(NOT_FOUND, "Issue 'PROJ-999' not found")`, exit 4
- [ ] No permission to transition: `CLIError(PERMISSION_DENIED, "No permission to transition PROJ-123")`, exit 5
- [ ] `--dry-run`: shows transition that would be executed including the matched target status, resolution, and comment if provided
- [ ] **Idempotency note (for agents):** `issue move` is NOT idempotent — moving to the current status may fail if no self-transition exists. Agents should check current status via `issue view --json` before moving.
- [ ] Tests: exact match, case-insensitive match, substring match, ambiguous substring, no match with alternatives, dry-run, `--resolution` with transition, `--comment` with transition, 404, 403

---

### US-026: Issue Assign Command

**Description:** As a developer, I want `jira issue assign PROJ-123 "Jane Doe"` to assign an issue, with `--unassign` to clear the assignee.

**Acceptance Criteria:**
- [ ] Positional args: issue key (required), user (required unless `--unassign`)
- [ ] User resolved via US-024 (ResolveUser)
- [ ] Calls `PUT /rest/api/3/issue/{key}/assignee` with `{"accountId": "..."}`
- [ ] `--unassign`: sends `{"accountId": null}`; if `--unassign` is provided alongside a user positional arg, `CLIError(VALIDATION_ERROR, "Cannot use --unassign with a user argument")`, exit 3
- [ ] Issue not found: `CLIError(NOT_FOUND, "Issue 'PROJ-999' not found")`, exit 4
- [ ] No permission: `CLIError(PERMISSION_DENIED, "No permission to assign PROJ-123")`, exit 5
- [ ] Success: `"Assigned PROJ-123 to Jane Doe"` or `"Unassigned PROJ-123"` to stdout
- [ ] `--json` output: `{"ok": true, "key": "PROJ-123", "action": "assigned", "assignee": {"account_id": "...", "display_name": "Jane Doe"}}` or `{"ok": true, "key": "PROJ-123", "action": "unassigned"}`
- [ ] `--dry-run`: validates issue exists and user resolves, shows preview
- [ ] **Idempotency note (for agents):** `issue assign` is idempotent — assigning to the same user again is safe.
- [ ] Tests: assign with name resolution, unassign, --unassign + user arg conflict, dry-run, user not found, 404, 403, JSON output shape

---

### US-027: Issue Delete Command

**Description:** As a developer, I want `jira issue delete PROJ-123 --yes` to delete an issue, with confirmation required to prevent accidents.

**Acceptance Criteria:**
- [ ] Positional arg: issue key (required)
- [ ] Requires `--yes` / `-y` flag (without it: `CLIError(VALIDATION_ERROR, "Use --yes to confirm deletion")`, exit 3)
- [ ] Calls `DELETE /rest/api/3/issue/{key}?deleteSubtasks=true`
- [ ] Success: `"Deleted PROJ-123"` to stdout
- [ ] `--json` output: `{"ok": true, "key": "PROJ-123", "action": "deleted"}`
- [ ] Issue not found: `CLIError(NOT_FOUND, "Issue 'PROJ-999' not found")`, exit 4
- [ ] No permission: `CLIError(PERMISSION_DENIED, "No permission to delete PROJ-123")`, exit 5
- [ ] `--dry-run`: validates issue exists and user has permission (GET issue first), shows preview
- [ ] **Idempotency note (for agents):** `issue delete` is NOT idempotent — deleting an already-deleted issue returns 404. Agents should handle 404 gracefully on retry.
- [ ] Tests: successful delete, missing --yes, 404, 403, dry-run, JSON output shape

---

### US-028: Issue List Command

**Description:** As a developer, I want `jira issue list --project PROJ --status "In Progress"` to list issues matching filter criteria, composing JQL from flags when `--jql` isn't provided.

**Acceptance Criteria:**
- [ ] Filter flags: `--project` (falls back to `default.project` from config), `--assignee` (resolved, `@me` supported), `--status`, `--type`, `--label`, `--sort` (string, e.g., `created`, `updated`, `priority`, `status`; default: none), `--order` (`asc` or `desc`; default: `desc`; only valid with `--sort`), `--limit` (default 50), `--offset` (default 0)
- [ ] `--jql` — raw JQL, overrides all filter flags (`--project`, `--assignee`, `--status`, `--type`, `--label`, `--sort`, `--order`) when provided. `--fields` is NOT overridden by `--jql` (it controls output field selection, not the query itself). `--limit` and `--offset` are also independent of `--jql`.
- [ ] JQL composition from flags (when `--jql` absent): join with AND, order: project, type, status, assignee, label. E.g., `--project PROJ --status "To Do"` → `project = "PROJ" AND status = "To Do"`. When `--sort` is provided, append `ORDER BY {field} {order}` (e.g., `--sort created --order asc` → `ORDER BY created ASC`)
- [ ] `--assignee @me` → `assignee = currentUser()` in JQL (no resolution needed)
- [ ] `--assignee "Jane"` → resolve to accountId, then `assignee = "{accountId}"`
- [ ] `--fields f1,f2` — controls which fields are returned from the API and displayed; default: `summary,status,assignee,priority,issuetype`; passed directly to the search request's `fields` array
- [ ] Search via `POST /rest/api/3/search/jql` with fields from `--fields` or the default set
- [ ] Table output: key + requested fields (summary truncated to terminal width, status with color, assignee as display name, priority as name)
- [ ] `--json`: full issue objects with pagination envelope
- [ ] No filters and no `--jql`: defaults to `assignee = currentUser() AND resolution = Unresolved` (same as `--assignee @me` behavior) — developer-friendly default matching `gh issue list` convention
- [ ] `--order` without `--sort`: `CLIError(VALIDATION_ERROR, "--order requires --sort")`, exit 3
- [ ] Empty results: table output prints `"No issues found"` to stdout; JSON output returns `{"data": [], "pagination": {"offset": 0, "limit": 50, "total": null, "has_next_page": false}}`
- [ ] Tests: filter composition, JQL override (filter flags + `--sort` ignored), `--fields` respected with `--jql`, assignee @me, assignee resolution failure (ambiguous, not found), pagination, empty results, default-to-mine behavior, `--sort` / `--order` appends ORDER BY, `--order` without `--sort` error

---

### US-029: Issue Transitions Command

**Description:** As a developer or LLM agent, I want `jira issue transitions PROJ-123` to list available transitions so I know what status changes are possible before attempting a move.

**Acceptance Criteria:**
- [ ] Calls `GET /rest/api/3/issue/{key}/transitions`
- [ ] Table output: transition name, target status, transition ID
- [ ] `--json`: list envelope `{"data": [{"id": "5", "name": "Start Progress", "to": {"name": "In Progress", "category": "In Progress"}}], "pagination": null}` — uses the standard list envelope for consistency (pagination is null since transitions are not paginated)
- [ ] Empty transitions: table output prints `"No transitions available for PROJ-123"` to stdout; JSON output returns `{"data": [], "pagination": null}`
- [ ] Issue not found: `CLIError(NOT_FOUND)`, exit 4
- [ ] Tests: successful list, empty transitions (text and JSON), 404

---

### US-030: Issue API Methods

**Description:** As a developer building issue commands, I need typed API methods on the Client for all issue operations so that commands don't construct raw HTTP requests.

**Acceptance Criteria:**
- [ ] `internal/api/issues.go` implements on `Client`:
  - `GetIssue(ctx, key string, opts GetIssueOptions) (*Issue, error)` — `opts` has Fields, Expand, Comments bool
  - `CreateIssue(ctx, input *CreateIssueInput) (*CreatedIssue, error)` — `input` has Project, Type, Summary, Description (ADF), Assignee, Priority, Labels, Parent, CustomFields; expects HTTP 201 response
  - `EditIssue(ctx, key string, input *EditIssueInput) error` — `input` has Fields map and Update operations; expects HTTP 204 (no response body)
  - `DeleteIssue(ctx, key string) error` — sends `deleteSubtasks=true` (string query param, not boolean); expects HTTP 204
  - `AssignIssue(ctx, key string, accountID *string) error` — nil = unassign; expects HTTP 204
  - `GetTransitions(ctx, key string) ([]Transition, error)`
  - `GetCreateMeta(ctx, projectKey string) (*CreateMetaIssueTypes, error)` — for `--dry-run` validation on issue create
  - `DoTransition(ctx, key string, input *DoTransitionInput) error` — `input` has TransitionID (required), Resolution (*string, optional), Comment (*adf.Document, optional)
  - `SearchIssues(ctx, jql string, opts SearchOptions) (*SearchResults, error)` — `opts` has Fields, Limit, PageToken
- [ ] `internal/api/users.go` implements:
  - `GetMyself(ctx) (*User, error)`
  - `SearchUsers(ctx, query string, opts PaginationOptions) ([]User, error)`
- [ ] All methods use `Client.Do()` internally
- [ ] Proper error wrapping: API errors → `CLIError` with context
- [ ] Tests: each method with `httptest` mock returning realistic Jira response shapes

---

### US-031: Search Command

**Description:** As a developer or LLM agent, I want `jira search "project = PROJ AND status = Open"` as a dedicated JQL search surface, separate from `issue list`, with `--mine` convenience shortcut — so that raw JQL and flag-based filtering have distinct, discoverable commands.

**Acceptance Criteria:**
- [ ] Positional arg: JQL string (optional — required unless `--mine` provided)
- [ ] `--mine` — shortcut for `assignee = currentUser() AND resolution = Unresolved`; when combined with `--status`, appends `AND status = "{status}"`
- [ ] `--status <status>` — only valid with `--mine`; ignored if positional JQL is provided. `--status` without `--mine` and without positional JQL: `CLIError(VALIDATION_ERROR, "--status requires --mine or a JQL query")`, exit 3
- [ ] `--fields f1,f2` — controls returned fields; default: `summary,status,assignee,priority,issuetype`
- [ ] `--limit` (default 50), `--offset` (default 0) — pagination
- [ ] `--json` — full issue objects with pagination envelope
- [ ] If positional JQL provided, `--mine` and `--status` are silently ignored (JQL takes precedence)
- [ ] If neither JQL nor `--mine`: `CLIError(VALIDATION_ERROR, "Provide a JQL query or use --mine")`, exit 3
- [ ] Search via `POST /rest/api/3/search/jql`
- [ ] Table output: key, summary (truncated), status, assignee, priority (same as issue list)
- [ ] Empty results: table output prints `"No issues found"` to stdout; JSON output returns `{"data": [], "pagination": {"offset": 0, "limit": 50, "total": null, "has_next_page": false}}`
- [ ] Tests: raw JQL, `--mine`, `--mine --status`, JQL overrides --mine, no args error, `--status` without `--mine` error, --fields, pagination, empty results

---

### US-032: Issue Key and ID Validation

**Description:** As a developer, I want the CLI to accept both issue keys (`PROJ-123`) and issue IDs (`10001`) as positional arguments, with client-side format validation to catch typos before making API calls.

**Acceptance Criteria:**
- [ ] Shared utility in `internal/cmd/issue/validate.go`: `ValidateIssueKeyOrID(input string) (string, error)`
- [ ] Accepts issue keys: 1+ ASCII uppercase letters, hyphen, 1+ digits (e.g., `PROJ-123`, `AB-1`). Jira Cloud restricts project keys to ASCII uppercase letters only — no Unicode/non-ASCII characters. Lowercase input (`proj-123`) is uppercased automatically to `PROJ-123`
- [ ] Accepts numeric issue IDs: all digits (e.g., `10001`) — passed through as-is (Jira API accepts both)
- [ ] Rejects anything else: `CLIError(VALIDATION_ERROR, "Invalid issue key or ID 'foo-bar-123'. Expected format: PROJ-123 or numeric ID")`, exit 3
- [ ] Applied in every command that takes an issue key positional arg: view, edit, move, assign, delete, transitions
- [ ] Tests: valid keys, lowercase normalization, numeric IDs, invalid formats (spaces, multiple hyphens, no digits)

---

## Functional Requirements

- FR-1: The CLI must compile to a single static binary named `jira` (via `go build ./cmd/jira`)
- FR-2: All input must be via flags and positional arguments — no interactive prompts, no TUI
- FR-3: Credentials resolve in order: flags → env vars → stored profile; partial credentials at any level produce a validation error
- FR-4: API tokens stored in OS keyring via `go-keyring`; plaintext fallback with stderr warning when keyring unavailable
- FR-5: Instance URLs normalized to bare hostname (strip protocol, trailing slash, API path suffix)
- FR-6: All errors implement `CLIError` with code, message, context, and suggestion; `main.go` is the sole error renderer
- FR-7: `--json` flag (explicit) > `output.format` config > default (`text`). `--text` flag overrides config-level JSON back to text. `--json` + `--text` is a validation error. `--jq` implies `--json`.
- FR-7a: `--jq <expr>` applies a jq expression (via `itchyny/gojq`) to JSON output before writing to stdout. Output is raw text, not JSON-wrapped. Requires `--json` or auto-enables it. `--jq` + `--text` is a validation error.
- FR-7b: `--quiet` / `-q` suppresses stdout on successful mutating commands (exit 0 is sufficient signal). Incompatible with `--json` and `--jq`.
- FR-8: Data to stdout, errors/warnings to stderr; no ANSI escape codes when stdout is not a TTY
- FR-9: Exit codes: 0=success, 1=general, 2=auth, 3=validation, 4=not-found, 5=permission, 6=rate-limited, 7=network-error, 8=conflict
- FR-10: `--dry-run` on mutating issue commands makes validation API calls (createmeta, user search) but not the mutating call itself. Auth commands (`login`, `logout`, `switch`) silently ignore `--dry-run`.
- FR-11: Destructive commands (`delete`, `logout`) require `--yes` / `-y` flag (not `--confirm` — per `gh` CLI convention, `--yes` is clearer in intent than `--confirm` which can be misread as "please confirm before proceeding")
- FR-12: Markdown input (descriptions, comments) converted to ADF via goldmark parser before API submission
- FR-13: Search uses `POST /rest/api/3/search/jql` with token-based pagination; `--offset` implemented as skip-N-results
- FR-14: `issue list` without `--jql` composes JQL from filter flags joined with AND; with no flags at all, defaults to `assignee = currentUser() AND resolution = Unresolved`
- FR-15: `--jql` takes precedence — all other filter flags ignored when present
- FR-16: User resolution algorithm: `@me` → /myself, account-ID-like → direct use, otherwise → search with exact-1-match requirement
- FR-17: Transition matching: case-insensitive exact match first, then case-insensitive substring match (single match only; 2+ = ambiguous error)
- FR-18: HTTP retries on 429 (respecting Retry-After) and 5xx with exponential backoff, max 3 retries; 30s request timeout. Never retry: 401 (abort with AUTH_ERROR), request timeouts (already waited 30s), other 4xx
- FR-18a: API client accepts HTTP 200, 201, and 204 as success. On 204, skip response body decoding. `POST /issue` returns 201; `PUT /issue` and `DELETE /issue` return 204.
- FR-19: Auth-free commands (`config`, `alias`, `meta commands`, `--help`, `--version`) must never trigger credential resolution
- FR-20: The `update` field operations (add/remove labels) and `fields` direct-set must never overlap in the same API call
- FR-21: `--project` falls back to `default.project` from config when omitted; if neither flag nor config is set, error
- FR-22: `--field key=value` always sends values as JSON strings; no client-side type coercion (Jira coerces server-side)
- FR-23: `--body-file -` rejects TTY stdin immediately; all stdin reads capped at 10MB. These rules apply to every command accepting `--body-file` (issue create, issue edit, and future comment add/edit). The 10MB limit also applies to `--description` flag content.
- FR-23a: `User.EmailAddress` may be null due to Jira privacy settings. All commands displaying or serializing email must handle null gracefully (omit from text, output `null` in JSON).
- FR-24: `--dry-run` on `issue edit` fetches the current issue first and shows a full before/after field diff
- FR-25: `--assignee` falls back to `default.assignee` from config when omitted on `issue create`; `@me` and display names are valid config values
- FR-26: Issue key positional args accept both keys (`PROJ-123`) and numeric IDs (`10001`); lowercase keys uppercased automatically; invalid formats rejected client-side before API call
- FR-27: `jira search` is a dedicated top-level command for raw JQL; `jira issue list` is for flag-based filtering. Both use the same `POST /rest/api/3/search/jql` endpoint.
- FR-28: Every mutating command produces structured `--json` output with `{"ok": true, ...}` envelope. Every read command produces bare objects (single) or `{"data": [...], "pagination": {...}}` (lists). Exception: `--web` on read commands uses `{"ok": true, "url": "..."}` since it performs a browser-open action.
- FR-29: `--web` on `issue view` reads instance from stored config only — no API call, no credential validation. Works offline.
- FR-30: Issue view displays linked issues (read-only) and subtasks when present; link *creation* is deferred to v2
- FR-31: `issue move` supports `--resolution` (optional, for transitions to Done-category statuses) and `--comment` (optional, Markdown→ADF) in the transition payload
- FR-32: Cobra's `SuggestionsMinimumDistance` set to 2 on root command for typo suggestions
- FR-33: `--sort <field>` and `--order asc|desc` on `issue list` append `ORDER BY` clause to composed JQL. `--order` without `--sort` is a validation error. Both are ignored when `--jql` is provided.
- FR-34: Pager support via `JIRA_PAGER` > `PAGER` > system default. Active when stdout is a TTY and not in `--json` mode. Disabled by `--no-pager` flag (registered on commands that produce long output, not as a global persistent flag).

## Non-Goals

- No interactive prompts, TUI, or `--interactive` mode
- No Agile API (boards, sprints, epics) — deferred to v2
- No attachments, worklogs, or watch/unwatch — deferred to v2
- No issue link *creation/deletion* (blocks, duplicates, relates-to) — deferred to v2. Read-only display of existing links IS in scope (US-021)
- No bulk operations — deferred to v2
- No shell completions in this phase (Phase 5)
- No `goreleaser` / CI/CD pipeline (Phase 6)
- No `jira schema`, `jira meta`, `jira project`, `jira user` commands (Phase 4)
- No alias expansion at runtime (Phase 5 — this phase only stores aliases)
- No `--verbose` HTTP debug logging (Phase 5)
- No `--template` flag for Go template output formatting — deferred to Phase 5. Use `--jq` for now.
- No `--json <field-names>` (gh-style field selection in `--json` flag) — use `--fields` to control API-level field selection instead. The `--json` flag is boolean-only.
- No stdin pipe for positional args (e.g., `echo "PROJ-123" | jira issue view -`) — use `xargs` instead: `echo "PROJ-123" | xargs jira issue view`
- No batch/multi-key positional args (e.g., `jira issue view PROJ-123 PROJ-124`) — single-key-only for clear error attribution per issue. Use shell loops for batch operations.
- No `--idempotency-key` for create operations — deferred; idempotency notes documented per command instead
- No webhook registration
- No `--dry-run` on auth commands (`login`, `logout`, `switch`) — these are non-destructive configuration operations; `--dry-run` is silently ignored if passed (not an error, but has no effect)
- No fuzzy/levenshtein matching for transition names — substring match is the maximum fuzziness

## Technical Considerations

- **Go 1.23** minimum — use modern features (range-over-func if useful)
- **Cobra command tree** — each command follows `NewCmdXxx(f *factory.Factory) *cobra.Command` pattern with `XxxOptions` struct; no `init()`, no global state
- **Jira API v3** — use new endpoints (not deprecated ones): `POST /rest/api/3/search/jql` (not `GET /rest/api/3/search`), `GET /rest/api/3/issue/createmeta/{proj}/issuetypes` (not old createmeta)
- **Basic Auth** — `Authorization: Basic base64(email:apitoken)` via custom `RoundTripper`
- **HTTP status codes** — `POST /issue` returns 201 (not 200); `PUT /issue` and `DELETE /issue` return 204 (no body). The API client's `Do()` method must accept 200/201/204 as success and skip body decoding on 204.
- **ADF is mandatory** — Jira v3 requires ADF for `description` and comment `body`; plain strings rejected
- **Search pagination is token-based** — no `total` field in response; `--offset` requires consume-and-discard strategy
- **`deleteSubtasks` is a string enum** (`"true"`/`"false"`), not a boolean — per Swagger spec. Send as `?deleteSubtasks=true` query param string.
- **Assignee uses `accountId` only** — `name` and `key` fields are deprecated on Jira Cloud
- **`fields` response is dynamic** — use typed `IssueFields` struct for known fields, `json.RawMessage` for custom fields
- **Shared `--body-file` utility** — stdin safety (TTY rejection, 10MB cap) implemented once in a shared helper (`internal/cmd/shared/bodyfile.go`), reused by issue create, issue edit, and future comment commands
- **Shared issue key validation** — `ValidateIssueKeyOrID` called at the top of every issue subcommand's `RunE`, before any API call
- **`User.EmailAddress` is nullable** — Jira privacy settings can mask it. Use `*string` in the Go struct. All display code must handle nil (omit in text, output `null` in JSON).
- **`createmeta` dual array fields** — `GET /issue/createmeta/{project}/issuetypes` response contains both `.createMetaIssueType` (legacy) and `.issueTypes` (preferred). Always read `.issueTypes`.
- **Retry policy exclusions** — never retry 401 (credential problem, not transient) or request timeouts (already waited 30s). Only retry 429 and 5xx.
- **Config file safety** — use write-to-temp-then-rename pattern for config writes to prevent corruption from concurrent processes or crashes mid-write

## Success Metrics

- `go build ./...` compiles cleanly with zero warnings from `go vet`
- `go test ./...` passes with >80% line coverage on `internal/` packages
- Full auth lifecycle works: login → status (valid) → switch → status → logout
- Issue lifecycle works: create → view → edit → move → assign → delete
- Search returns results via both `jira search` (raw JQL) and `jira issue list` (flag-based)
- `--json` output parseable by `jq` for every command — mutation commands include `"ok": true`
- `--jq` inline filtering works for all commands producing JSON output (e.g., `jira issue list --json --jq '.[].key'`)
- Exit codes match spec for every error category (0-8)
- No ANSI codes in piped output (`jira issue view PROJ-123 | cat` is clean)
- Structured errors include correct context and suggestions for agent self-correction
- `jira issue move PROJ-123 progress` works (substring match to "In Progress")
- `jira issue move PROJ-123 Done --resolution Fixed` sets resolution atomically with transition
- Typo suggestions work: `jira isue view` → `Did you mean "issue"?`
- All tests use mocks — no external Jira instance required

## Resolved Decisions

1. **`jira issue list` with no flags** → defaults to `assignee = currentUser() AND resolution = Unresolved` (same as `--assignee @me`). Developer-friendly default matching `gh issue list` convention.
2. **`--field key=value` type coercion** → always send as JSON string; no client-side coercion. Jira coerces strings to the target type server-side for most fields. Revisit when Phase 4 (`jira schema fields`) provides field type metadata for smarter coercion.
3. **`--body-file -` (stdin) safety** → no timeout (would break legitimate pipes). Two guards: (a) if stdin is a TTY (not piped), error immediately with `"--body-file - requires piped input"`; (b) 10MB read cap to prevent OOM.
4. **Issue edit `--dry-run`** → full before/after diff. Fetches current issue via GET first, computes diff per changed field, displays `field: old → new`. Worth the extra API call for actionable output.
5. **`default.project` from config** → yes, respected. Resolution order: `--project` flag > `default.project` config > error. Applies to `issue create`, `issue list`, and any command taking `--project`.
6. **`jira search` vs `jira issue list`** → both exist as separate commands. `search` is the raw JQL power tool (positional arg). `issue list` is the flag-based ergonomic tool. Both share the same search API method and output formatting.
7. **Transition matching** → exact match first, then substring. No fuzzy/levenshtein — substring is the maximum fuzziness to avoid surprising behavior. Ambiguous substrings are an error. `--resolution` and `--comment` flags allow setting resolution and comment atomically with the transition.
8. **Issue key validation** → client-side, before API call. Lowercase auto-uppercased. Numeric IDs accepted. Invalid formats rejected with clear error.
9. **`--json` vs `output.format` config** → flag always wins. `--text` flag added for overriding config-level JSON back to text. No `--no-json`. `--jq` implies `--json`.
10. **`--confirm` renamed to `--yes`** → per `gh` CLI convention (Issue #6892). `--confirm` reads as "please confirm" not "I confirm." `--yes` / `-y` is unambiguous.
11. **`--jq` inline filtering** → added as global flag using `itchyny/gojq` (pure Go). Eliminates hard dependency on external `jq` for agents and scripts. Matches `gh --jq` behavior.
12. **`--sort` / `--order` on `issue list`** → added. Appends `ORDER BY` to composed JQL. Avoids forcing users to drop to `--jql` just to sort results.
13. **`--quiet` flag** → added as global flag for mutating commands. Suppresses success output (exit 0 is sufficient). Incompatible with `--json` and `--jq`.
14. **HTTP status codes** → API client accepts 200/201/204 as success. 204 skips body decoding. Verified against Swagger spec: `POST /issue` → 201, `PUT /issue` → 204, `DELETE /issue` → 204.
15. **Null email handling** → `User.EmailAddress` is `*string`. Jira privacy settings can mask it. Text output omits email; JSON outputs `null`.
16. **Retry exclusions** → never retry 401 (not transient) or request timeouts (already waited 30s). Only 429 and 5xx.

## Open Questions

None — all design questions resolved.
