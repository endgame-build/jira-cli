# Jira CLI — Functional Specification

## Context

Build a CLI tool that wraps the Jira Cloud Platform REST API v3 to give developers a fast, scriptable interface for daily Jira workflows. The tool must also serve as a first-class integration surface for LLM agents and CI/CD automation — any consumer should be able to discover available commands, introspect valid inputs for the current Jira instance, validate actions before executing, and self-correct from structured errors.

**Design constraints:**
- Strictly non-interactive — all inputs via args/flags, no prompts or TUI
- Pipeable/composable — structured output (`--json`), exit codes, stdin support
- Developer-first — optimize for the IC workflow
- Agent-first — machine-readable discovery, introspection, error guidance
- Implementation-independent — this spec defines *what*, not *how*

> **Note:** Where this spec and the PRD (`tasks/prd-jira-cli-foundation.md`) differ, the PRD supersedes. Key differences: PRD adds exit codes 7 (network-error) and 8 (conflict), adds `--text` flag, adds `--jq` flag, adds `--quiet` flag, renames `--confirm` to `--yes`, adds `--resolution`/`--comment` to `issue move`, adds `--sort`/`--order`/`--fields` to `issue list`, adds `--instance`/`--user`/`--token` global flags, and moves `--sort` from Future Scope into scope.

---

## 1. Authentication (`jira auth`)

### Authentication Resolution Order
Credentials are resolved in this order (first match wins):
1. **Flags** — `--instance`, `--user`, `--token` on any command
2. **Environment variables** — `JIRA_INSTANCE`, `JIRA_USER`, `JIRA_TOKEN`
3. **Active profile** — stored credentials from `jira auth login`

This allows CI/CD pipelines to use env vars without `jira auth login`, while developers use stored profiles.

### 1.1 `jira auth login`
Stores credentials for a Jira Cloud instance.

| Input | Required | Description |
|---|---|---|
| `--instance <url>` | yes | Jira Cloud instance (e.g. `mycompany.atlassian.net`) |
| `--user <email>` | yes | Atlassian account email |
| `--token <token>` | yes | API token |

**Behavior:**
- Validates credentials by calling `/rest/api/3/myself`
- Persists credentials securely (implementation decides where/how)
- Supports multiple named profiles: `--profile <name>` (default: `default`)
- Exit 0 on success, non-zero on auth failure

### 1.2 `jira auth status`
Shows current authentication state: instance URL, user email, profile name, token validity (live check). Supports `--json`.

### 1.3 `jira auth logout`
Removes stored credentials. Optional `--profile <name>`. Requires `--yes`.

### 1.4 `jira auth switch <profile>`
Sets the active profile.

---

## 2. Issues (`jira issue`)

### 2.1 `jira issue view <issue-key>`
Display a single issue.

**Output:** key, summary, status, assignee, reporter, priority, type, labels, description (truncated), created/updated dates, linked issues, subtasks.

| Flag | Description |
|---|---|
| `--fields <f1,f2,...>` | Show only specific fields |
| `--json` | Machine-readable JSON output |
| `--comments` | Include comments in output |
| `--web` | Open in default browser instead |

### 2.2 `jira issue create`

| Input | Required | Description |
|---|---|---|
| `--project <key>` | yes | Project key |
| `--type <type>` | yes | Issue type (Bug, Task, Story, etc.) |
| `--summary <text>` | yes | Issue summary/title |
| `--description <text>` | no | Issue description |
| `--assignee <user>` | no | Assignee (account ID, email, or display name — resolved via user search) |
| `--priority <name>` | no | Priority name |
| `--labels <l1,l2>` | no | Comma-separated labels |
| `--parent <key>` | no | Parent issue (for subtasks) |
| `--field <key=value>` | no | Set arbitrary/custom field (repeatable) |
| `--body-file <path>` | no | Read description from file (`-` for stdin) |
| `--dry-run` | no | Validate and show what would be created, don't execute |

**Output:** created issue key + URL. Exit 0 on success.

**Text input format:** Description and comment bodies accept **Markdown**. The CLI converts Markdown to Atlassian Document Format (ADF) before sending to the API. Plain text is also accepted as-is.

### 2.3 `jira issue edit <issue-key>`
Update fields on an existing issue.

| Input | Description |
|---|---|
| `--summary <text>` | New summary |
| `--description <text>` | New description |
| `--assignee <user>` | New assignee (`""` to unassign) |
| `--priority <name>` | New priority |
| `--labels <l1,l2>` | Set labels (replaces) |
| `--add-labels <l1,l2>` | Append labels |
| `--remove-labels <l1,l2>` | Remove specific labels |
| `--field <key=value>` | Set arbitrary/custom field (repeatable) |
| `--body-file <path>` | Read description from file/stdin |
| `--dry-run` | Validate and show diff, don't execute |

At least one field flag required.

### 2.4 `jira issue move <issue-key> <status>`
Transition an issue to a new workflow status.

**Behavior:**
- Resolves target status name to available transitions
- Exact match preferred; errors on ambiguity
- **On failure:** returns available transitions in structured format (name + target status) so agents can self-correct
- Supports `--dry-run`

### 2.5 `jira issue assign <issue-key> <user>`
Assign issue. `--unassign` flag to clear. Supports `--dry-run`.

### 2.6 `jira issue delete <issue-key>`
Delete an issue. Requires `--yes`. Supports `--dry-run` (validates issue exists and user has permission).

### 2.7 `jira issue list`

| Flag | Description |
|---|---|
| `--project <key>` | Filter by project |
| `--assignee <user>` | Filter by assignee (`@me` for self) |
| `--status <status>` | Filter by status |
| `--type <type>` | Filter by issue type |
| `--label <label>` | Filter by label |
| `--limit <n>` | Max results (default: 50) |
| `--offset <n>` | Pagination offset |
| `--jql <query>` | Raw JQL (overrides other filters) |
| `--json` | JSON output |

**Output:** tabular list — key, summary, status, assignee, priority. When `--jql` is absent, other flags compose into JQL.

### 2.8 `jira issue transitions <issue-key>`
List available transitions for an issue.

**Output:** transition name, target status, transition ID. Supports `--json`. This is a primary discovery endpoint for agents.

---

## 3. Comments (`jira comment`)

### 3.1 `jira comment list <issue-key>`
| Flag | Description |
|---|---|
| `--limit <n>` | Max results |
| `--offset <n>` | Pagination offset |
| `--json` | JSON output |

### 3.2 `jira comment add <issue-key>`
| Input | Required | Description |
|---|---|---|
| `--body <text>` | yes* | Comment body (Markdown) |
| `--body-file <path>` | yes* | Read from file (`-` for stdin) |
| `--dry-run` | no | Validate without posting |

*One of `--body` or `--body-file` required. Output: comment ID + permalink.

### 3.3 `jira comment edit <issue-key> <comment-id>`
Same flags as `add`. Supports `--dry-run`.

### 3.4 `jira comment delete <issue-key> <comment-id>`
Requires `--yes`. Supports `--dry-run`.

---

## 4. Search (`jira search`)

### 4.1 `jira search [jql]`

| Flag | Description |
|---|---|
| `--fields <f1,f2,...>` | Fields to return |
| `--limit <n>` | Max results (default: 50) |
| `--offset <n>` | Pagination offset |
| `--json` | JSON output |
| `--mine` | Shortcut: filter to current user's unresolved issues |
| `--status <status>` | Combine with `--mine` to filter by status |

**JQL argument is optional.** When omitted, flags like `--mine` generate the JQL. When provided, `--mine` and `--status` are **ignored** (explicit JQL takes precedence). Error if neither JQL nor `--mine` is provided.

**Convenience aliases:**

| Shortcut | JQL equivalent |
|---|---|
| `--mine` | `assignee = currentUser() AND resolution = Unresolved` |
| `--mine --status "X"` | adds `AND status = "X"` |

---

## 5. Projects (`jira project`)

### 5.1 `jira project list`
**Output:** key, name, lead, type. Supports `--json`, `--limit`, `--offset`.

### 5.2 `jira project view <project-key>`
**Output:** key, name, lead, description, issue types, URL. Supports `--json`.

---

## 6. Users (`jira user`)

### 6.1 `jira user search <query>`
Search for users by name, email, or username.

| Flag | Description |
|---|---|
| `--limit <n>` | Max results (default: 10) |
| `--offset <n>` | Pagination offset |
| `--json` | JSON output |

**Output:** account ID, display name, email, active status.

This is essential for agents — resolves human-readable names to the account IDs that the API requires.

### 6.2 `jira user me`
Show the authenticated user's info. Output: account ID, display name, email. Supports `--json`.

---

## 7. Schema Introspection (`jira schema`)

These commands expose the **dynamic shape** of the Jira instance so agents and automation can discover what's valid before acting.

### 7.1 `jira schema fields`
List all fields available in the instance.

| Flag | Description |
|---|---|
| `--project <key>` | Scope to project's field configuration |
| `--type <issue-type>` | Show fields for a specific issue type |
| `--json` | JSON output |

**Output:** field ID, name, type (string/number/array/etc.), required flag, allowed values (for select fields).

### 7.2 `jira schema types`
List issue types available.

| Flag | Description |
|---|---|
| `--project <key>` | Scope to a specific project |
| `--json` | JSON output |

**Output:** type name, description, subtask flag, icon URL.

### 7.3 `jira schema statuses`
List statuses and workflows.

| Flag | Description |
|---|---|
| `--project <key>` | Scope to project |
| `--type <issue-type>` | Scope to issue type |
| `--json` | JSON output |

**Output:** status name, category (To Do / In Progress / Done), ID.

### 7.4 `jira schema priorities`
List available priorities.

**Output:** name, ID, description, icon URL. Supports `--json`.

### 7.5 `jira schema labels`
List labels in use across the instance (or scoped to `--project`). Supports `--json`, `--limit`, `--offset`.

---

## 8. Command Metadata (`jira meta`)

Machine-readable CLI self-description for agents.

### 8.1 `jira meta commands`
List all available commands with their arguments, flags, types, and descriptions.

**Output (JSON by default):**
```
[
  {
    "command": "issue create",
    "description": "Create a new issue",
    "args": [],
    "flags": [
      {"name": "--project", "type": "string", "required": true, "description": "Project key"},
      {"name": "--type", "type": "string", "required": true, "description": "Issue type"},
      ...
    ]
  },
  ...
]
```

This is the **primary entry point for agents** — call once to understand the full CLI surface.

### 8.2 `jira meta version`
Output version info + API compatibility.

```
{"version": "0.1.0", "api": "jira-cloud-v3", "instance": "mycompany.atlassian.net"}
```

---

## 9. Configuration (`jira config`)

### 9.1 `jira config set <key> <value>`
**Keys:**
- `default.project` — default `--project` for commands
- `default.assignee` — default assignee (`@me`)
- `output.format` — `text` or `json`
- `output.color` — `auto`, `always`, `never`

### 9.2 `jira config get <key>`

### 9.3 `jira config list`
Show all configuration values. Supports `--json`.

### 9.4 `jira alias set <name> <command>`
User-defined aliases (e.g. `jira alias set mine "search --mine"`).

### 9.5 `jira alias list`
Supports `--json`.

---

## Global Flags (all commands)

| Flag | Description |
|---|---|
| `--profile <name>` | Use a specific auth profile |
| `--json` | Force JSON output |
| `--text` | Force text output (overrides `output.format = json` config) |
| `--jq <expr>` | Apply jq expression to JSON output (implies `--json`) |
| `--quiet` / `-q` | Suppress success output on mutating commands |
| `--no-color` | Disable color |
| `--verbose` | Debug output |
| `--dry-run` | Validate without executing (mutating commands only) |
| `--instance <url>` | Jira Cloud instance (overrides env/profile) |
| `--user <email>` | Atlassian account email (overrides env/profile) |
| `--token <token>` | API token (overrides env/profile) |
| `--help` | Command help |
| `--version` | Version |

---

## Output & Error Conventions

### Output
1. **Tabular text** by default — aligned columns, human-readable
2. **JSON** with `--json` — full structured data, suitable for `jq` and agents
3. **Errors to stderr**, data to stdout
4. **No color** when stdout is not a TTY (pipe-safe)

### Pagination
All list/search commands support `--limit <n>` and `--offset <n>`. JSON output includes pagination metadata:
```json
{"data": [...], "pagination": {"offset": 0, "limit": 50, "total": 237}}
```

### Exit Codes
| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | General error |
| 2 | Authentication error |
| 3 | Validation error (bad input, missing required flags) |
| 4 | Not found |
| 5 | Permission denied |
| 6 | Rate limited (retry-after in structured error) |
| 7 | Network error (connection refused, DNS failure, TLS error, timeout) |
| 8 | Conflict (409 concurrent modification) |

### Structured Errors (Agent-Friendly)
All errors include structured context when `--json` is active:

```json
{
  "error": {
    "code": "INVALID_TRANSITION",
    "message": "Status 'QA' is not reachable from current status 'Open'",
    "context": {
      "current_status": "Open",
      "available_transitions": [
        {"name": "Start Progress", "target": "In Progress"},
        {"name": "Close", "target": "Closed"}
      ]
    },
    "suggestion": "Use one of the available transitions listed above"
  }
}
```

Error patterns for agent self-correction:
- **Invalid field value** → includes allowed values
- **Invalid transition** → includes available transitions
- **Unknown issue type** → includes valid types for the project
- **Missing required field** → includes field name and type
- **Permission denied** → includes required permission name
- **Rate limited** → includes retry-after seconds

### Dry-Run Output
`--dry-run` on mutating commands outputs what would happen without executing:

```json
{
  "dry_run": true,
  "action": "create_issue",
  "payload": {
    "project": "PROJ",
    "type": "Bug",
    "summary": "Login fails on Safari",
    "fields": {"priority": "High"}
  },
  "validation": "ok"
}
```

---

## Core Workflows

| Workflow | Commands |
|---|---|
| Morning standup prep | `jira search --mine --status "In Progress"` |
| Start working on issue | `jira issue move PROJ-123 "In Progress"` |
| Quick update | `jira comment add PROJ-123 --body "PR merged"` |
| Close from CI | `jira issue move PROJ-123 Done` |
| Create from template | `jira issue create --project PROJ --type Bug --summary "..." --body-file report.md` |
| Pipe to scripts | `jira search --mine --json \| jq '.[] \| .key'` |

### Agent Workflow (LLM)
1. `jira meta commands` → discover CLI surface
2. `jira schema types --project PROJ --json` → discover valid issue types
3. `jira schema fields --project PROJ --type Bug --json` → discover required/optional fields
4. `jira issue create --project PROJ --type Bug --summary "..." --dry-run --json` → validate
5. `jira issue create --project PROJ --type Bug --summary "..."` → execute
6. On error → parse structured error → self-correct → retry

### Automation Workflow (CI/CD)
1. `jira schema statuses --project PROJ --type Story --json` → discover valid statuses
2. `jira issue transitions PROJ-123 --json` → discover available transitions for this issue
3. `jira issue move PROJ-123 "Done" --dry-run --json` → validate transition
4. `jira issue move PROJ-123 "Done"` → execute

---

## Future Scope (v2)

Explicitly deferred — not blocking core or agentic workflows:

- **Issue links** — create/remove links (blocks, duplicates, relates-to)
- **Attachments** — upload/download files on issues
- **Worklogs** — log and list time entries
- **Watching** — watch/unwatch issues
- **Bulk operations** — move/edit multiple issues via JQL filter
- **Agile API** — boards, sprints, epics, backlog (separate Jira Software API)
- **Webhooks** — register/manage webhooks for event-driven automation

---

## Verification

The spec is complete when:
1. Every command is expressible entirely through args/flags (no prompts)
2. Every read command has `--json` output
3. Every mutating command supports `--dry-run`
4. Every destructive command requires `--yes`
5. All errors include structured context for self-correction (with `--json`)
6. An agent can go from zero knowledge → full workflow using only `jira meta` + `jira schema`
7. All output is pipe-safe (data stdout, messages stderr)
8. Multiple instances supported via profiles
