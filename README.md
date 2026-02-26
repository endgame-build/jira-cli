# jira-cli

Non-interactive CLI for Jira Cloud REST API v3 — built for developers, agents, and CI/CD.

```
jira issue create --project PROJ --type Bug --summary "Login fails on Safari"
jira issue view PROJ-123 --json
jira issue move PROJ-123 "In Progress"
jira search "project = PROJ AND status = Open" --limit 10
```

## Features

- **Non-interactive** — all input via flags, no prompts, no TUI
- **Pipe-safe** — structured JSON output, correct exit codes, no ANSI when piped
- **Agent-friendly** — structured errors with codes, context, and suggestions for LLM self-correction
- **Three credential sources** — flags, environment variables, or stored profiles (keyring-backed)
- **Markdown input** — descriptions and comments written in Markdown, converted to Atlassian Document Format
- **JQL support** — raw JQL via `jira search` or flag-based filtering via `jira issue list`
- **Inline filtering** — `--jq` flag for extracting fields without external tools

## Install

```
go install github.com/endgameio/jira-cli/cmd/jira@latest
```

## Quick Start

```sh
# Authenticate
jira auth login --instance mycompany.atlassian.net --user me@company.com --token <api-token>

# View an issue
jira issue view PROJ-123

# Create an issue
jira issue create --project PROJ --type Task --summary "Add retry logic" --description "Handle 429 responses"

# Edit an issue
jira issue edit PROJ-123 --add-labels urgent --priority High

# Transition an issue
jira issue move PROJ-123 "In Progress"

# Assign an issue
jira issue assign PROJ-123 "Jane Doe"

# Search with JQL
jira search "project = PROJ AND assignee = currentUser()" --json

# List my open issues
jira issue list --assignee @me
```

## Configuration

Config stored at `$XDG_CONFIG_HOME/jira-cli/config.toml` (`~/.config/jira-cli/` on Linux, `~/Library/Application Support/jira-cli/` on macOS). Set defaults to skip repetitive flags:

```sh
jira config set default.project PROJ
jira config set output.format json
```

## CI/CD Usage

```sh
export JIRA_INSTANCE=mycompany.atlassian.net
export JIRA_USER=ci@company.com
export JIRA_TOKEN=$JIRA_API_TOKEN

jira issue create --project PROJ --type Bug --summary "Build failed" --json
```

## Error Codes

| Exit | Meaning |
|------|---------|
| 0 | Success |
| 1 | General error |
| 2 | Authentication error |
| 3 | Validation error |
| 4 | Not found |
| 5 | Permission denied |
| 6 | Rate limited |
| 7 | Network error |
| 8 | Conflict |

## Status

Under active development. Targeting Jira Cloud REST API v3.

## License

Proprietary.
