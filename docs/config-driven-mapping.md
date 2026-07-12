# Config-driven field mapping (`--map`)

By default, `jira issue import` expects markdown frontmatter using the CLI's own keys
(`key`, `summary`, `type`, `priority`, …). When your documents use a different vocabulary — e.g. a
planning hub where an epic carries `name`, `jira_key`, `jira_issue_type`, `initiative`, and `stream` —
pass a declarative field-map with `--map` and the CLI translates those documents into the canonical
model, reusing the same create/update/ADF/conflict pipeline.

`--map` is **opt-in and backward compatible**: without it, behavior is unchanged.

See [`jira-sync.example.yaml`](./jira-sync.example.yaml) for a fully annotated example.

## Push content — document → Jira

```sh
jira issue import ./epics/**/*.md --map ./jira-sync.yaml --force
```

- `name` → summary, body → ADF description, `priority` → the project's priority (via `priority_map`),
  `stream` → a `stream:*` label (derived from the id prefix), and the parent is resolved from the
  document's `initiative` (epic) or `parent_epic_jira_key` (story) per the `links` mechanism.
- A document without a `jira_key` is **created**; after create, the assigned key and `last_synced_at`
  are **written back** into the source file so re-runs update instead of re-creating.
- `status` and `assignee` are **never pushed** — they are Jira-first (see below).

## Pull state — Jira → document

```sh
jira issue pull ./epics/**/*.md --map ./jira-sync.yaml
```

For each document with a real `jira_key`, this writes back **only** the fields in `pull.fields`
(`status`, `assignee`) — mapping the Jira status to your vocabulary (by category, then explicit name)
and the assignee as an email or account id. All other frontmatter and the body are left untouched.
This is the reverse of `import --map` and is safe to run on a schedule.

## Deletion

Deletion is intentionally **not** inferred by scanning Jira. Drive it from your own source of truth:
when a document is removed, read its `jira_key` (e.g. from the pre-change git state in CI) and call
`jira issue delete <key>` on those exact keys.
