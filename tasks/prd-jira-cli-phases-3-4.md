# PRD: Jira CLI Phases 3–4 (Comments, Schema, Meta, Projects, Users)

## Introduction

Extend the Jira CLI with the remaining read/write command surfaces defined in SPEC.md sections 3–9. Phase 3 delivers comment CRUD — the last write-heavy feature set. Phase 4 delivers the agent discovery and introspection layer: schema inspection, command metadata, project browsing, and user lookup. Together they complete the CLI's functional coverage — after these phases, every command in the spec has an implementation.

This PRD covers Phases 3–4 from the implementation plan. Phase 3 adds comment management. Phase 4 adds schema introspection (`jira schema`), command metadata (`jira meta`), project commands (`jira project`), and user commands (`jira user`). Search (`jira search`), config, and alias CRUD are already shipped in Phases 0–2 and excluded from this PRD.

> **Note:** Where this PRD and the SPEC or PLAN differ, this PRD supersedes.

## SPEC Deviations

This section explicitly documents where the PRD diverges from SPEC.md:

| Area | SPEC.md says | This PRD says | Rationale |
|------|-------------|---------------|-----------|
| `user search --offset` | `--offset` flag listed | **Not supported** (FR-46) | Jira's `startAt` on plain-array user search responses is unreliable; only `--limit` exposed |
| `schema fields --project/--type` | Implies server-side filtering | Flags accepted as **forward-compat no-ops** with stderr warning (FR-39) | `GET /field` returns all fields globally; no Jira API supports scoped filtering |
| `schema statuses --project/--type` | Implies server-side filtering | Flags accepted as **forward-compat no-ops** with stderr warning (US-046) | `GET /status` returns all statuses globally; precise filtering requires `issue transitions` |
| `schema labels --project` | Implies scoping | Flag accepted as **forward-compat no-op** with stderr warning (US-048) | `GET /label` is global; Jira API has no project-scoped label endpoint |
| `meta commands` JSON | List shape implied | **Bare array** — no `{"data":..., "pagination":...}` envelope (FR-54) | CLI metadata, not Jira data; no pagination applies |

## Goals

- Deliver full comment lifecycle (list, add, edit, delete) with Markdown→ADF, `--body-file`, and `--dry-run` support
- Expose the agent discovery surface: `jira meta commands` for CLI introspection and `jira schema *` for Jira instance introspection
- Enable the full agent workflow: `meta commands` → `schema types` → `schema fields` → `issue create --dry-run` → `issue create`
- Add project browsing and user lookup as supporting commands for both humans and agents
- Register all new command groups on the root command (`comment`, `project`, `user`, `schema`, `meta`)

## JSON Envelope Reference

All `--json` output conforms to one of five shapes. Commands reference these by name in their acceptance criteria.

| Shape | When used | Example commands |
|-------|-----------|-----------------|
| **Offset-paginated list** `{"data":[...], "pagination":{"offset":0, "limit":25, "total":42, "has_next_page":true}}` | List endpoints with known total | `comment list`, `project list`, `schema labels` |
| **Raw-array list** `{"data":[...], "pagination":{"offset":0, "limit":10, "total":null, "has_next_page":true}}` | Plain-array endpoints; `total` unknown, `has_next_page` inferred from `len(results)==limit` | `user search` |
| **Unpaginated list** `{"data":[...], "pagination":null}` | Endpoints returning complete arrays (no pagination) | `schema fields`, `schema types`, `schema statuses`, `schema priorities` |
| **Bare object** `{...fields...}` | Single-item view commands | `issue view`, `project view`, `user me`, `meta version` |
| **Bare array** `[{...}, ...]` | CLI metadata (not Jira data, no pagination concept) | `meta commands` |
| **Mutation** `{"ok":true, "action":"added", ...}` | All write operations | `comment add`, `comment edit`, `comment delete` |
| **Dry-run** `{"dry_run":true, "payload":{...}, "validation":"..."}` | `--dry-run` on mutations | `comment add --dry-run`, `comment edit --dry-run`, `comment delete --dry-run` |

## User Stories

---

### US-033: Comment List Command

**Description:** As a developer, I want `jira comment list PROJ-123` to show comments on an issue so I can review discussion without opening a browser.

**Acceptance Criteria:**
- [ ] Positional arg: issue key (required, validated via `ValidateIssueKeyOrID`)
- [ ] Calls `GET /rest/api/3/issue/{key}/comment` with `orderBy=-created` (newest first), `startAt`, `maxResults`
- [ ] Flags: `--limit` (default 25), `--offset` (default 0), `--json`, `--no-pager`
- [ ] Table output: each comment shows comment ID, author display name, created date (relative — see US-053), body (first 3 lines, truncated with `...`)
- [ ] Comment body displayed as plaintext — use `ToPlaintext()` from `internal/adf` to render ADF bodies (US-038)
- [ ] `--json`: offset-paginated list envelope — comments endpoint returns `total`, so include it. Timestamps in JSON are ISO 8601 (raw Jira values), never relative strings.
- [ ] Empty comments: table output prints `"No comments on PROJ-123"` to stdout; JSON returns `{"data": [], "pagination": {"offset": 0, "limit": 25, "total": 0, "has_next_page": false}}`
- [ ] Issue not found: `CLIError(NOT_FOUND, "Issue 'PROJ-999' not found")`, exit 4
- [ ] Pager enabled when stdout is TTY and `--json` is not active (same as `issue view`)
- [ ] Tests: successful list with mock response, pagination, empty comments (text and JSON), 404, relative date formatting, ADF→plaintext rendering, JSON timestamps are ISO 8601

---

### US-034: Comment Add Command

**Description:** As a developer, I want `jira comment add PROJ-123 --body "PR merged, closing."` to post a comment so I can update issues from the terminal or CI.

**Acceptance Criteria:**
- [ ] Positional arg: issue key (required, validated)
- [ ] Flags: `--body` (string, Markdown→ADF), `--body-file` (path, `-` for stdin), `--dry-run`, `--json`, `--quiet`
- [ ] One of `--body` or `--body-file` required; if neither: `CLIError(VALIDATION_ERROR, "Provide --body or --body-file")`, exit 3
- [ ] `--body-file` uses the shared `internal/cmd/shared/bodyfile.go` utility (TTY rejection, 10MB cap) — already built in Phase 2
- [ ] `--body-file` overrides `--body` if both given (same as `issue create`)
- [ ] Calls `POST /rest/api/3/issue/{key}/comment` with ADF body; expects HTTP 201
- [ ] Success output: `"Added comment {commentId} to PROJ-123: https://{instance}/browse/PROJ-123?focusedCommentId={commentId}"` to stdout (permalink constructed client-side from instance + issue key + comment ID — see Technical Considerations for fragility note)
- [ ] `--json` output: `{"ok": true, "key": "PROJ-123", "comment_id": "10042", "action": "added", "url": "https://{instance}/browse/PROJ-123?focusedCommentId=10042"}`
- [ ] `--dry-run`: validates issue exists (GET issue), shows preview of the comment body that would be posted; JSON mode: `{"dry_run": true, "key": "PROJ-123", "payload": {"body": "..."}, "validation": "passed (issue exists)"}`
- [ ] Issue not found: `CLIError(NOT_FOUND, "Issue 'PROJ-999' not found")`, exit 4
- [ ] No permission: `CLIError(PERMISSION_DENIED, "No permission to comment on PROJ-123")`, exit 5
- [ ] HTTP 413: parse Jira error response body; if it indicates per-issue comment limit, return `CLIError(VALIDATION_ERROR, "Comment limit exceeded for PROJ-123")`, exit 3; otherwise return `CLIError(VALIDATION_ERROR, "Request too large")`, exit 3
- [ ] **Idempotency note (for agents):** `comment add` is NOT idempotent — retrying creates duplicate comments. Agents should use `--dry-run` first, then add, and check the returned comment ID to confirm success.
- [ ] Tests: successful add, body-file from stdin, dry-run, missing body flag, 404, 413 (both limit and oversized), JSON output shape

---

### US-035: Comment Edit Command

**Description:** As a developer, I want `jira comment edit PROJ-123 10042 --body "Updated: PR merged"` to update a comment.

**Acceptance Criteria:**
- [ ] Positional args: issue key (required, validated), comment ID (required)
- [ ] Flags: `--body` (string, Markdown→ADF), `--body-file` (path), `--dry-run`, `--json`, `--quiet`
- [ ] One of `--body` or `--body-file` required; same validation as `comment add`
- [ ] Calls `PUT /rest/api/3/issue/{key}/comment/{id}` with ADF body; expects HTTP 200 (returns updated comment)
- [ ] Success output: `"Updated comment {commentId} on PROJ-123"` to stdout
- [ ] `--json` output: `{"ok": true, "key": "PROJ-123", "comment_id": "10042", "action": "updated"}`
- [ ] `--dry-run`: validates issue and comment exist (GET comment), shows preview of what would change; JSON mode: `{"dry_run": true, "key": "PROJ-123", "comment_id": "10042", "payload": {"body": "..."}, "validation": "passed (comment exists)"}`
- [ ] Comment not found: `CLIError(NOT_FOUND, "Comment '99999' not found on PROJ-123")`, exit 4
- [ ] Issue not found: `CLIError(NOT_FOUND, "Issue 'PROJ-999' not found")`, exit 4
- [ ] No permission: `CLIError(PERMISSION_DENIED, "No permission to edit comment on PROJ-123")`, exit 5
- [ ] **Idempotency note (for agents):** `comment edit` is idempotent — setting the same body again is safe.
- [ ] Tests: successful edit, body-file, dry-run, missing body flag, comment 404, issue 404, JSON output shape

---

### US-036: Comment Delete Command

**Description:** As a developer, I want `jira comment delete PROJ-123 10042 --yes` to remove a comment, with confirmation required to prevent accidents.

**Acceptance Criteria:**
- [ ] Positional args: issue key (required, validated), comment ID (required)
- [ ] Flags: `--yes` / `-y` (required), `--dry-run`, `--json`, `--quiet`
- [ ] Requires `--yes` / `-y` flag; without it: `CLIError(VALIDATION_ERROR, "Use --yes to confirm deletion")`, exit 3. `--dry-run` bypasses the `--yes` requirement (dry-run is non-destructive by definition — same pattern as `issue delete`)
- [ ] Calls `DELETE /rest/api/3/issue/{key}/comment/{id}`; expects HTTP 204
- [ ] Success output: `"Deleted comment {commentId} from PROJ-123"` to stdout
- [ ] `--json` output: `{"ok": true, "key": "PROJ-123", "comment_id": "10042", "action": "deleted"}`
- [ ] `--dry-run`: validates issue and comment exist (GET comment), shows what would be deleted (comment ID, author, created date, first line of body)
- [ ] Comment not found: `CLIError(NOT_FOUND, "Comment '99999' not found on PROJ-123")`, exit 4
- [ ] No permission: `CLIError(PERMISSION_DENIED, "No permission to delete comment on PROJ-123")`, exit 5
- [ ] **Idempotency note (for agents):** `comment delete` is NOT idempotent — deleting an already-deleted comment returns 404. Agents should handle 404 gracefully on retry.
- [ ] Tests: successful delete, missing --yes, comment 404, dry-run, JSON output shape

---

### US-037: Comment API Methods

**Description:** As a developer building comment commands, I need typed API methods on the Client for all comment operations.

**Acceptance Criteria:**
- [ ] `internal/api/comments.go` implements on `Client`:
  - `ListComments(ctx, issueKey string, opts OffsetPaginationOptions) (*CommentPage, error)` — sends `orderBy=-created`, `startAt`, `maxResults`; returns full `CommentPage` with `StartAt`, `MaxResults`, `Total`, `Comments`
  - `AddComment(ctx, issueKey string, body *adf.Document) (*Comment, error)` — POST, expects 201
  - `UpdateComment(ctx, issueKey, commentID string, body *adf.Document) (*Comment, error)` — PUT, expects 200
  - `DeleteComment(ctx, issueKey, commentID string) error` — DELETE, expects 204
  - `GetComment(ctx, issueKey, commentID string) (*Comment, error)` — GET single comment (for dry-run validation)
- [ ] `OffsetPaginationOptions` type added to `internal/api/types.go`: `StartAt int`, `MaxResults int` — distinct from existing token-based `PaginationOptions` to avoid confusion
- [ ] All methods use `Client.Do()` internally
- [ ] Proper error wrapping: 404 → NOT_FOUND with issue key / comment ID in context, 403 → PERMISSION_DENIED, 413 → VALIDATION_ERROR (parse response body to distinguish per-issue limit from oversized payload)
- [ ] Tests: each method with `httptest` mock returning realistic Jira response shapes (201, 200, 204, 404, 403, 413)

---

### US-038: ADF to Plaintext Converter

**Description:** As a developer, I need to display ADF content (comment bodies, descriptions) as readable plaintext in the terminal so that text output is human-friendly.

**Acceptance Criteria:**
- [ ] `internal/adf/plaintext.go` adds `ToPlaintext(doc json.RawMessage) string` alongside the existing `ExtractText()` function. **`ExtractText` is preserved** — it serves a different purpose (raw text concatenation for truncated previews in `issue list`). `ToPlaintext` produces structured plaintext with formatting.
- [ ] Signature returns `string` only (no error) — graceful degradation on all inputs:
  - Empty or null ADF input → empty string
  - Invalid JSON → return raw string as fallback
  - Valid ADF → structured plaintext per mapping below
- [ ] Mapping: paragraph→text+newline, heading→text+newline, bulletList→`- item` per line, orderedList→`1. item` per line, codeBlock→indented 4 spaces, blockquote→`> ` prefix, rule→`---`, hardBreak→newline
- [ ] Inline marks: bold/italic/strike→stripped (text only), code→backtick-wrapped, link→`text (url)` format
- [ ] Nested lists handled (indent by 2 spaces per level)
- [ ] Tests: table-driven with ADF JSON input → expected plaintext output; cover each node type, nested lists, inline marks, empty input, malformed input
- [ ] **Enhancement:** Update Phase 2's `issue view` table output to use `ToPlaintext()` for description rendering (replacing raw `ExtractText()` truncation). JSON output unchanged (pass-through ADF). Test: `issue view` with complex ADF description renders structured plaintext.

---

### US-039: Project List Command

**Description:** As a developer or agent, I want `jira project list` to browse available projects so I can discover project keys for other commands.

**Acceptance Criteria:**
- [ ] Calls `GET /rest/api/3/project/search` with `startAt`, `maxResults`
- [ ] Flags: `--limit` (default 50), `--offset` (default 0), `--json`
- [ ] Table output: key, name, lead (display name), type (projectTypeKey)
- [ ] `--json`: offset-paginated list envelope
- [ ] Response uses `values` array (not `projects`) — mapped correctly from `PageBeanProject` shape
- [ ] Empty results: `"No projects found"` text; JSON returns empty data array
- [ ] Tests: successful list, pagination, empty results, JSON output shape

---

### US-040: Project View Command

**Description:** As a developer or agent, I want `jira project view PROJ` to see project details including available issue types.

**Acceptance Criteria:**
- [ ] Positional arg: project key or ID (required)
- [ ] Calls `GET /rest/api/3/project/{keyOrId}`
- [ ] Flags: `--json`
- [ ] Table output: key, name, lead (display name), description (truncated to 5 lines via `ToPlaintext()` if ADF, raw truncation otherwise), type, issue types (comma-separated list of names), URL (`https://{instance}/browse/{key}`)
- [ ] `--json`: bare object output (no envelope — single item, same as `issue view`)
- [ ] Project not found: `CLIError(NOT_FOUND, "Project 'XYZ' not found")`, exit 4
- [ ] Tests: successful view, 404, JSON output shape, issue types display

---

### US-041: Project API Methods

**Description:** As a developer building project commands, I need typed API methods for project operations.

**Acceptance Criteria:**
- [ ] `internal/api/projects.go` implements on `Client`:
  - `ListProjects(ctx, opts OffsetPaginationOptions) (*ProjectSearchResult, error)` — GET `/project/search` with `startAt`, `maxResults`
  - `GetProject(ctx, keyOrID string) (*ProjectDetail, error)` — GET `/project/{keyOrId}` — accepts both project keys (`PROJ`) and numeric IDs (`10001`)
- [ ] New types in `internal/api/types.go`:
  - `ProjectDetail` — full project: `ID`, `Key`, `Name`, `Description`, `Lead` (*User), `ProjectTypeKey`, `IssueTypes` ([]IssueType), `URL`, `Simplified`, `Style`
  - `ProjectSearchResult` — `Values` ([]ProjectDetail), `StartAt`, `MaxResults`, `Total`, `IsLast`
- [ ] Tests: each method with mock responses

---

### US-042: User Search Command

**Description:** As a developer or agent, I want `jira user search "jane"` to find users so I can resolve names to account IDs for assignment and other operations.

**Acceptance Criteria:**
- [ ] Positional arg: search query (required)
- [ ] Calls `GET /rest/api/3/user/search?query={input}` with `maxResults`
- [ ] Flags: `--limit` (default 10), `--json`
- [ ] Table output: account ID, display name, email (or `(hidden)` when null), active status
- [ ] `--json`: raw-array list envelope — `total` is null; `has_next_page` inferred from `len(results) == limit` (via existing `FetchRawArrayPage` from Phase 2)
- [ ] Empty results: `"No users matching 'xyz'"` text; JSON returns empty data array
- [ ] `--offset` is **not** supported — the Jira user search API's `startAt` behavior is unreliable for plain-array responses. Only `--limit` is exposed. (SPEC deviation — see SPEC Deviations table above.)
- [ ] Tests: successful search, empty results, null email handling, JSON output shape

---

### US-043: User Me Command

**Description:** As a developer or agent, I want `jira user me` to show my authenticated user info so I can verify my identity and get my account ID.

**Acceptance Criteria:**
- [ ] Calls `GET /rest/api/3/myself` (already built in Phase 2 for auth)
- [ ] Flags: `--json`
- [ ] Table output: account ID, display name, email (or `(hidden)` when null)
- [ ] `--json`: bare object output — the full `User` struct as returned by the Jira API: `{"accountId": "...", "displayName": "...", "emailAddress": "..." | null, "active": true, ...}`. Field names use Jira API camelCase (matching Go struct json tags), not snake_case.
- [ ] Auth failure: standard `CLIError(AUTH_ERROR)`, exit 2
- [ ] Tests: successful output, null email, JSON output shape

---

### US-044: Schema Fields Command

**Description:** As an agent, I want `jira schema fields` to discover all available fields in the Jira instance so I know what `--field` keys are valid for issue create/edit.

**Acceptance Criteria:**
- [ ] Calls `GET /rest/api/3/field` — returns plain array (no pagination)
- [ ] Flags: `--project` (optional, forward-compat no-op), `--type` (optional, forward-compat no-op), `--json`
- [ ] Table output: field ID, name, type (`schema.type`, e.g., "string", "array", "number"), custom flag (ID starts with `customfield_`)
- [ ] `--json`: unpaginated list envelope
- [ ] `--project` and `--type` flags: accepted for forward compatibility but **do not change the returned data**. Produce warning to stderr: `"Note: Field filtering by project/type requires the Field Configuration API (not yet implemented). Showing all fields."`. Flag help text explicitly states: `"(not yet implemented — shows all fields)"`
- [ ] **Agent guidance:** The `GET /issue/createmeta/{project}/issuetypes` endpoint (already used by `issue create --dry-run`) returns fields scoped to a project+type. Agents needing scoped fields should use `issue create --dry-run --project P --type T`.
- [ ] Tests: successful list, JSON output shape, project/type warning message emitted to stderr, returned data identical with and without flags

---

### US-045: Schema Types Command

**Description:** As an agent, I want `jira schema types` to discover valid issue types so I can populate the `--type` flag on `issue create`.

**Acceptance Criteria:**
- [ ] Without `--project`: calls `GET /rest/api/3/issuetype` — returns all issue types the user can see (plain array)
- [ ] With `--project`: must resolve project key to numeric ID first via `GET /rest/api/3/project/{key}` (the `/issuetype/project` endpoint requires numeric `projectId`), then calls `GET /rest/api/3/issuetype/project?projectId={id}` — returns issue types scoped to that project
- [ ] Flags: `--project` (optional), `--json`
- [ ] Table output: name, description (truncated), subtask flag (yes/no), icon URL
- [ ] `--json`: unpaginated list envelope — each item includes `id`, `name`, `description`, `subtask`, `iconUrl`, `hierarchyLevel`, `scope`
- [ ] Project not found (when `--project` given): `CLIError(NOT_FOUND, "Project 'XYZ' not found")`, exit 4
- [ ] Tests: global types, project-scoped types, project 404, JSON output shape

---

### US-046: Schema Statuses Command

**Description:** As an agent, I want `jira schema statuses` to discover valid workflow statuses so I can determine what targets are available for `issue move`.

**Acceptance Criteria:**
- [ ] Calls `GET /rest/api/3/status` — returns all statuses (plain array)
- [ ] Flags: `--project` (optional, forward-compat no-op), `--type` (optional, forward-compat no-op), `--json`
- [ ] Table output: name, category name (To Do / In Progress / Done), ID
- [ ] `--json`: unpaginated list envelope
- [ ] `--project` and `--type` flags: accepted for forward compatibility but **do not change the returned data**. Produce warning to stderr: `"Note: Status filtering by project/type is not yet implemented. Showing all statuses. Use 'jira issue transitions <key>' for issue-specific available transitions."`. Flag help text states: `"(not yet implemented — shows all statuses)"`
- [ ] Tests: successful list, JSON output shape, filtering warning emitted to stderr, returned data identical with and without flags

---

### US-047: Schema Priorities Command

**Description:** As an agent, I want `jira schema priorities` to discover valid priority names so I can populate `--priority` on `issue create/edit`.

**Acceptance Criteria:**
- [ ] Calls `GET /rest/api/3/priority` — returns all priorities (plain array)
- [ ] Flags: `--json`
- [ ] Table output: name, ID, description (truncated), icon URL
- [ ] `--json`: unpaginated list envelope — each item includes `id`, `name`, `description`, `iconUrl`, `statusColor`, `isDefault`
- [ ] Tests: successful list, JSON output shape

---

### US-048: Schema Labels Command

**Description:** As an agent, I want `jira schema labels` to discover labels in use so I can populate `--labels` on issue commands.

**Acceptance Criteria:**
- [ ] Calls `GET /rest/api/3/label` with `startAt`, `maxResults` — offset-based pagination
- [ ] Flags: `--project` (optional, forward-compat no-op), `--limit` (default 50), `--offset` (default 0), `--json`
- [ ] Table output: label names, one per line
- [ ] `--json`: offset-paginated list envelope — data is string array: `{"data": ["bug", "frontend", ...], "pagination": {"offset": 0, "limit": 50, "total": 120, "has_next_page": true}}`
- [ ] Response shape: `PageBeanString` with `values` array of strings — mapped to the standard list envelope
- [ ] `--project` flag: accepted for forward compatibility but **does not change the returned data**. Produces warning to stderr: `"Note: Label scoping by project is not supported by the Jira API. Showing all labels."`. Flag help text states: `"(not supported by Jira API — shows all labels)"`
- [ ] Tests: successful list, pagination, empty results, JSON output shape, project warning emitted to stderr

---

### US-049: Schema API Methods

**Description:** As a developer building schema commands, I need typed API methods for all introspection endpoints.

**Acceptance Criteria:**
- [ ] `internal/api/schema.go` implements on `Client`:
  - `ListFields(ctx) ([]Field, error)` — GET `/field`, plain array
  - `ListIssueTypes(ctx) ([]IssueType, error)` — GET `/issuetype`, plain array
  - `ListIssueTypesForProject(ctx, projectID string) ([]IssueType, error)` — GET `/issuetype/project?projectId={id}`, plain array
  - `ListStatuses(ctx) ([]StatusDetail, error)` — GET `/status`, plain array
  - `ListPriorities(ctx) ([]Priority, error)` — GET `/priority`, plain array
  - `ListLabels(ctx, opts OffsetPaginationOptions) (*LabelPage, error)` — GET `/label`, offset-based
- [ ] New types in `internal/api/types.go`:
  - `Field` — `ID`, `Key`, `Name`, `Schema` (FieldSchema), `Custom` (bool, derived from ID prefix)
  - `FieldSchema` — `Type`, `Items` (for array fields), `Custom` (custom field type URI), `System` (system field name)
  - `StatusDetail` — embeds `Status`, adds `Description` (string), `IconURL` (string). Used only by `ListStatuses` / `schema statuses`; existing `Status` type is unchanged for issue fields and transitions.
  - `LabelPage` — `Values` ([]string), `StartAt`, `MaxResults`, `Total`, `IsLast`
- [ ] **Phase 2 type enhancements required** — the following existing types need additional fields for Phase 3–4 JSON output:
  - `Priority` (currently has `ID`, `Name`, `IconURL`) — add `Description string`, `StatusColor string`, `IsDefault bool` (needed by US-047 JSON output)
  - `IssueType` (currently has `ID`, `Name`, `Description`, `Subtask`, `IconURL`) — add `HierarchyLevel int`, `Scope json.RawMessage` (needed by US-045 JSON output). Adding these fields is backward-compatible — `json.Unmarshal` ignores missing fields.
- [ ] **`StatusDetail` replaces `Status` in schema context:** `ListStatuses` returns `[]StatusDetail`. `StatusDetail` embeds `Status` and adds `Description string`, `IconURL string`. This avoids modifying the existing `Status` type used throughout the codebase (issue fields, transitions). The `schema statuses` command uses `StatusDetail`; other commands continue using `Status`.
- [ ] Tests: each method with mock responses

---

### US-050: Meta Commands Command

**Description:** As an LLM agent, I want `jira meta commands` to discover the entire CLI surface so I can build valid commands without documentation.

**Acceptance Criteria:**
- [ ] Walks the Cobra command tree at runtime using `root.Commands()` recursion
- [ ] Extracts from each command: full command path (e.g., "issue create"), `Use` string, `Short` description, all flags (name, type, required flag from Cobra's `MarkFlagRequired`, default value, description)
- [ ] Positional args extracted from `Use` field or `Args` validator annotation
- [ ] Output is JSON by default (even without `--json` flag — this is a machine-first command). `--json` flag is accepted but is a no-op (already JSON). `--jq` works as expected (filters the JSON output).
- [ ] JSON shape: **bare array** (NOT the list envelope — this is CLI metadata, not Jira data, so the `{"data": [...], "pagination": ...}` envelope does not apply). See FR-54 for rationale.
  ```json
  [
    {
      "command": "issue create",
      "description": "Create a new issue",
      "args": [{"name": "issue-key", "required": true, "description": "..."}],
      "flags": [
        {"name": "--project", "type": "string", "required": true, "default": "", "description": "Project key"},
        {"name": "--type", "type": "string", "required": true, "default": "", "description": "Issue type"}
      ]
    }
  ]
  ```
- [ ] Hidden commands and flags excluded (Cobra's `Hidden` field)
- [ ] Text output (with `--text`): table with columns: COMMAND (full path), DESCRIPTION (`Short`), FLAGS (comma-separated names of required flags, or `"(none)"` if no required flags). Example row: `issue create | Create a new issue | --project, --type, --summary`
- [ ] This command works WITHOUT authentication (no API calls) — added to the auth-free list in Factory
- [ ] Tests: output includes known commands (issue create, auth login, etc.), hidden commands excluded, JSON is valid and parseable, --text table format, no auth triggered

---

### US-051: Meta Version Command

**Description:** As an agent or script, I want `jira meta version` to check CLI version and API compatibility.

**Acceptance Criteria:**
- [ ] Output: `{"version": "0.1.0", "api": "jira-cloud-v3", "instance": "mycompany.atlassian.net"}` — `instance` is null when no profile is configured (this command does NOT require auth)
- [ ] JSON by default (machine-first, same as `meta commands`)
- [ ] Version injected via ldflags (already set up in Phase 2 `main.go`)
- [ ] `instance` read from stored config/profile if available, null otherwise — no API call, no credential validation
- [ ] Text output (with `--text`): `"jira version 0.1.0 (jira-cloud-v3)\nInstance: mycompany.atlassian.net"` or `"jira version 0.1.0 (jira-cloud-v3)\nInstance: (not configured)"` when no profile
- [ ] This command works WITHOUT authentication
- [ ] Tests: JSON output shape, version populated, instance null when no profile, instance populated when profile exists

---

### US-052: Register Phase 3–4 Command Groups

**Description:** As a developer, I need the new command groups registered on the root command so they're discoverable via `--help` and `jira meta commands`.

**Acceptance Criteria:**
- [ ] Root command (`internal/cmd/root/root.go`) registers five new subcommand groups: `comment`, `project`, `user`, `schema`, `meta`
- [ ] Each group has a parent command with `Short` and `Long` descriptions (e.g., `jira comment` shows help for comment subcommands)
- [ ] `meta` commands (`meta commands`, `meta version`) added to the auth-free command list — Factory never triggers credential resolution for them
- [ ] `schema`, `project`, `user`, and `comment` commands DO require authentication (they make API calls)
- [ ] Cobra help output shows all groups organized logically
- [ ] Tests: `--help` on root includes new groups, `meta` commands don't trigger auth resolution

---

### US-053: Relative Time Formatter

**Description:** As a developer, I need a relative time formatter for displaying timestamps as "2 hours ago", "3 days ago", etc. in table output.

**Acceptance Criteria:**
- [ ] `internal/output/time.go` implements `RelativeTime(iso8601 string) string` — parses ISO 8601 timestamp and returns relative string
- [ ] Thresholds:
  - < 1 minute → `"just now"`
  - < 60 minutes → `"X minutes ago"` (singular: `"1 minute ago"`)
  - < 24 hours → `"X hours ago"` (singular: `"1 hour ago"`)
  - < 30 days → `"X days ago"` (singular: `"1 day ago"`, `"yesterday"` for 1 day)
  - < 12 months → `"X months ago"` (singular: `"1 month ago"`)
  - ≥ 12 months → `"X years ago"` (singular: `"1 year ago"`)
- [ ] Parse failure → return the raw input string unchanged (graceful degradation)
- [ ] No external dependencies — uses `time.Parse` and `time.Since` thresholds
- [ ] Used by `comment list` table output; NOT used in `--json` output (JSON always has raw ISO 8601 timestamps)
- [ ] Tests: table-driven covering each threshold boundary, singular/plural, parse failure fallback

---

### US-054: User API Methods

**Description:** As a developer building user commands, I need typed API methods for user endpoints.

**Acceptance Criteria:**
- [ ] `internal/api/users.go` — both methods **already exist** from Phase 2. Verify they meet Phase 3–4 requirements:
  - `GetMyself(ctx) (*User, error)` — GET `/myself` ✓ already exists and returns full `User` struct
  - `SearchUsers(ctx, query string, startAt, maxResults int) ([]User, error)` — GET `/user/search` ✓ already exists with `startAt` parameter. The command layer (US-042) simply passes `startAt=0` and does not expose `--offset` to the user. The `startAt` parameter is retained in the API method for potential future use; the unreliability caveat (FR-46) applies at the CLI flag level, not the API method level.
- [ ] Verify error wrapping: 401 → AUTH_ERROR, 429 → handled by retry transport
- [ ] Add tests if not already present: empty search results, null email in response, GetMyself auth failure

---

## Functional Requirements

- FR-35: Comment bodies are always ADF. The CLI converts Markdown→ADF on write (add/edit) and ADF→plaintext on read (list). Shared `--body-file` utility from Phase 2 is reused.
- FR-36: `comment delete` requires `--yes` / `-y` (same pattern as `issue delete`, `auth logout`)
- FR-37: Comment list uses offset-based pagination with `total` included in the envelope (unlike search which is token-based)
- FR-38: `POST /issue/{key}/comment` returns 201; `PUT` returns 200; `DELETE` returns 204. HTTP status handling follows Phase 2 conventions.
- FR-39: `jira schema fields` returns all fields globally. `--project`/`--type` are accepted for forward compatibility but **do not change the returned data** — they only emit a stderr warning. Precise scoping requires createmeta (already available via `issue create --dry-run`).
- FR-40: `jira schema types --project PROJ` requires resolving the project key to a numeric ID via `GET /project/{key}` before calling `GET /issuetype/project?projectId={id}`. The `issuetype/project` endpoint does not accept project keys.
- FR-41: Schema endpoints that return plain arrays (`fields`, `types`, `statuses`, `priorities`) use `"pagination": null` in their JSON list envelope to signal no pagination.
- FR-42: `jira schema labels` is the only schema endpoint with pagination (offset-based via `GET /label`).
- FR-43: `jira meta commands` and `jira meta version` output JSON by default (no `--json` flag needed). Use `--text` to get human-readable output.
- FR-44: `jira meta commands` extracts command metadata from Cobra's runtime tree — no static registry. This means it's always accurate and zero-maintenance.
- FR-45: `jira user search` returns a plain array from the API — `total` is null in pagination metadata, `has_next_page` inferred from `len(results) == limit`.
- FR-46: `jira user search` does not support `--offset` — only `--limit`. The Jira user search API's `startAt` behavior is unreliable for plain-array responses. (SPEC deviation.)
- FR-47: Auth-free commands expanded to include `meta commands` and `meta version` (in addition to existing `config`, `alias`, `--help`, `--version`).
- FR-48: All new mutation commands (`comment add`, `comment edit`, `comment delete`) produce `{"ok": true, ...}` JSON output, consistent with Phase 2 mutation conventions.
- FR-49: HTTP 413 from comment endpoints: parse the Jira error response body to distinguish per-issue comment limits from oversized payloads. Per-issue limit → `CLIError(VALIDATION_ERROR, "Comment limit exceeded for {key}")`. Oversized payload → `CLIError(VALIDATION_ERROR, "Request too large")`. Both exit 3.
- FR-50: `comment delete --dry-run` bypasses the `--yes` requirement — dry-run is non-destructive. This aligns with the existing `issue delete --dry-run` behavior (already implemented in Phase 2).
- FR-51: HTTP 403 on comment operations mapped to `CLIError(PERMISSION_DENIED)`, exit 5. Jira returns 403 when the user lacks permission to add/edit/delete comments on the issue.
- FR-52: All mutation JSON output includes an `action` field for agent disambiguation: `"added"`, `"updated"`, `"deleted"` for comments. Consistent with Phase 2 patterns (`"moved"`, `"assigned"`, `"deleted"` for issues).
- FR-53: `jira user me` and `jira user search` JSON output uses Jira API camelCase field names (`accountId`, `displayName`, `emailAddress`) — not snake_case. Consistent with `issue view --json` which passes through Jira's field naming.
- FR-54: `jira meta commands` JSON output is a **bare array** `[{...}, ...]`, not the standard list envelope. Rationale: this is CLI metadata (always complete, never paginated, not Jira data). The list envelope exists to communicate pagination state; applying it to a non-paginated, non-API response would mislead agent consumers into looking for pagination tokens that will never exist.
- FR-55: Forward-compat no-op flags (`--project`, `--type` on schema commands that don't support them) must: (a) not change the returned data, (b) emit a warning to stderr, (c) include `"(not yet implemented)"` or `"(not supported)"` in their `--help` description. This prevents agents from assuming the flag works.
- FR-56: JSON timestamps in all list/view output are raw ISO 8601 strings from the Jira API. Relative time formatting ("2 hours ago") is applied only in table/text output, never in JSON. This ensures agents get machine-parseable timestamps.
- FR-57: Comment list JSON output includes full `Comment` objects with `id`, `author` (nested User), `body` (raw ADF JSON), `created`, `updated` — not the plaintext-converted body. The plaintext conversion is a display concern, not a data concern.
- FR-58: The `OffsetPaginationOptions` type (`StartAt int`, `MaxResults int`) is used by all new offset-based API methods (comments, projects, labels). This is distinct from the existing token-based `PaginationOptions` (`MaxResults int`, `NextPageToken string`) to prevent confusion.
- FR-59: Comment dry-run JSON output includes context fields (`key`, `comment_id`) alongside the standard `dry_run`, `payload`, and `validation` fields. The current `OutputDryRun(payload, validation, tableFn)` formatter produces `{"dry_run":true, "payload":..., "validation":...}`. To add extra context fields, either: (a) enhance `OutputDryRun` to accept an optional `extras map[string]interface{}` parameter, or (b) embed `key`/`comment_id` inside the `payload` object. Option (a) is preferred for agent consistency — context fields at the top level make it easy for agents to identify the target resource without parsing the payload. Implementation detail deferred to the implementer.
- FR-60: Phase 2 type field additions (`Priority.Description`, `Priority.StatusColor`, `Priority.IsDefault`, `IssueType.HierarchyLevel`, `IssueType.Scope`) are backward-compatible — `json.Unmarshal` ignores missing JSON fields, and `json.Marshal` omits zero-value fields with `omitempty`. No Phase 2 commands break.

## Non-Goals

- No comment visibility/restriction support (`visibility` field) — comments are always public within the project. Deferred to v2.
- No `comment add` via inline positional arg (e.g., `jira comment add PROJ-123 "text"`) — `--body` flag only, matching SPEC.md and keeping the flag-only convention
- No `--expand renderedBody` for HTML rendering of comments — plaintext conversion is sufficient for terminal output
- No project create/edit/delete — read-only project commands only
- No user create/invite — read-only user commands only
- No `schema fields --project --type` precise scoping via createmeta (fields endpoint returns all fields globally; createmeta is available via `issue create --dry-run` as a workaround)
- No `schema statuses --project --type` precise workflow scoping (use `jira issue transitions <key>` for issue-specific transitions)
- No field value enumeration in `schema fields` output (allowed values for select fields require the createmeta endpoint, not the fields endpoint)
- No `jira meta commands` static registry or hand-maintained metadata — purely Cobra-introspected
- No `--verbose` HTTP debug logging — deferred to Phase 5
- No shell completions — deferred to Phase 5
- No alias expansion — deferred to Phase 5

## Technical Considerations

- **ADF→plaintext converter** (`internal/adf/plaintext.go`): `ToPlaintext()` is added alongside existing `ExtractText()`. `ExtractText()` does raw text concatenation (used for issue list summary truncation); `ToPlaintext()` produces structured plaintext with bullets, indentation, code blocks, etc. (used for comment bodies and issue view descriptions). Both coexist — they serve different purposes.
- **Relative time formatting** (`internal/output/time.go`): `RelativeTime()` converts ISO 8601 → "2 hours ago", "3 days ago", etc. Used only in table output; JSON always returns raw ISO 8601. See US-053 for thresholds.
- **Project key→ID resolution** for `schema types --project` adds an extra API call. Cache the resolved ID within the command invocation (no persistent caching needed — commands are short-lived).
- **`GET /field` returns all fields** — typically 200-400 fields on an active Jira instance. No pagination needed but output can be large. Table output should be compact; `--json` output is unlimited.
- **Cobra command introspection** for `meta commands`: use `cmd.Commands()` recursion, `cmd.Flags().VisitAll()`, `cmd.HasAvailableFlags()`. Flag "required" status is tracked by Cobra's `MarkFlagRequired` annotation (`cobra.BashCompOneRequiredFlag`).
- **`Comment.Body` is `json.RawMessage`** (already defined in Phase 2 types) — the ADF→plaintext converter takes raw JSON input.
- **User search rate limiting**: the Jira user search endpoint can return 429. The existing retry transport from Phase 2 handles this automatically.
- **Permalink URL construction** for `comment add` (`?focusedCommentId={id}`): constructed client-side from instance hostname + issue key + comment ID. This is fragile if Atlassian changes URL structure, but there's no API-provided permalink. The `self` URL in comment responses is a REST API URL, not a browser URL. Accept this as a known fragility — the URL is a convenience, not a critical path.
- **`OffsetPaginationOptions` vs `PaginationOptions`**: Phase 2 defined `PaginationOptions` for token-based search pagination (`MaxResults`, `NextPageToken`). Phase 3–4 endpoints use offset-based pagination (`StartAt`, `MaxResults`). A new `OffsetPaginationOptions` type avoids overloading the existing type with unrelated fields.
- **`OutputDryRun` enhancement needed (FR-59):** Comment dry-run output needs `key` and `comment_id` at the JSON top level (alongside `dry_run`, `payload`, `validation`). Phase 2's `OutputDryRun(payload, validation, tableFn)` doesn't support extra fields. Recommended fix: add an `OutputDryRunWithContext(extras map[string]interface{}, payload, validation, tableFn)` variant, or refactor `OutputDryRun` to accept optional extras. This is a minor formatter enhancement, not a new output shape.
- **Phase 2 type field additions (FR-60):** `Priority` gains `Description`, `StatusColor`, `IsDefault`; `IssueType` gains `HierarchyLevel`, `Scope`. These are additive — existing Phase 2 commands that use these types are unaffected because `omitempty` prevents serialization of zero-value fields, and deserialization ignores missing fields. Run Phase 2 tests after the type changes to confirm.
- **`StatusDetail` embedding pattern:** `StatusDetail` embeds the existing `Status` struct and adds `Description`, `IconURL`. This keeps the `Status` type stable for issue fields and transitions while giving `schema statuses` the richer fields from `GET /status`. The embedding means `StatusDetail` inherits `ID`, `Name`, `StatusCategory` from `Status` — no duplication.

## Success Metrics

- `go build ./...` compiles cleanly with zero warnings from `go vet`
- `go test ./...` passes with >80% line coverage on new packages
- Comment lifecycle works: add → list → edit → delete
- Full agent workflow executes: `meta commands` → `schema types --project PROJ` → `schema fields` → `issue create --dry-run` → `issue create`
- `jira meta commands` output is valid JSON that includes all registered commands with args and flags
- `jira meta commands --text` renders a readable table
- `jira schema types --project PROJ` returns project-scoped issue types
- `jira project list` and `jira user search` return correctly formatted results
- All `--json` outputs parseable by `jq` and match the envelope shapes in the JSON Envelope Reference table
- Exit codes match spec for all new error paths
- No ANSI codes in piped output for any new command
- All tests use mocks — no external Jira instance required
- Forward-compat no-op flags emit warnings to stderr and do not alter returned data

## Open Questions

None — all design questions resolved during clarification.
