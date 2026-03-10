# Plan: Add `--tree` flag to `jira issue export`

## Context

The `jira issue export` command currently produces a flat file layout: `PROJECT/KEY - Summary.md`. The plan files in `oncierge-hub` use a hierarchical structure where epics are folders containing `_epic.md` + story files. We need export to produce a matching structure so round-tripping (export from Jira -> edit locally -> import back) preserves the folder-based organization.

## Target output

```
PROJECT/
├── PROJ-1 - Epic Name/
│   ├── _epic.md                    # epic content
│   ├── PROJ-10 - Story One.md     # child of PROJ-1
│   └── PROJ-11 - Story Two.md    # child of PROJ-1
├── PROJ-2 - Another Epic/
│   ├── _epic.md
│   └── PROJ-20 - Some Task.md
└── PROJ-50 - Orphan Issue.md      # no parent → flat at project root
```

## Key insight

No buffering/reordering needed. Every child issue already carries `issue.Fields.Parent.Key` and `issue.Fields.Parent.Fields.Summary` inline from the Jira API. Streaming write-as-you-go works.

## Changes

### 1. `internal/markdown/filemap.go` — Add `IssueTreePath()`

Extract `projectKey` logic into helper `issueProjectKey(issue)` (reused by both `IssuePath` and new function).

New function `IssueTreePath(issue api.Issue) string`:
- **Epic** (`strings.EqualFold(issue.Fields.IssueType.Name, "epic")`) → `PROJECT/{Key} - {Summary}/_epic.md`
- **Has parent** (`issue.Fields.Parent != nil && Key != ""`) → `PROJECT/{ParentKey} - {ParentSummary}/{Key} - {Summary}.md`
- **Otherwise** → `PROJECT/{Key} - {Summary}.md` (same as flat)

All names go through existing `SanitizeFilename()`.

### 2. `internal/cmd/issue/export.go` — Wire `--tree` flag

- Add `Tree bool` to `ExportOptions`
- Add `cmd.Flags().BoolVar(&opts.Tree, "tree", false, "Organize output hierarchically (epics as directories)")` in `NewCmdExport`
- Update `Long` description to mention tree layout: `"With --tree, epics become directories containing _epic.md and their child issues."`
- In `runExport`, change path resolution:
  ```go
  // line ~106
  var relPath string
  if opts.Tree {
      relPath = markdown.IssueTreePath(issue)
  } else {
      relPath = markdown.IssuePath(issue)
  }
  ```

No other changes — `writeFileAtomic` already calls `os.MkdirAll` so nested dirs are created automatically.

### 3. `internal/markdown/filemap_test.go` — Unit tests for `IssueTreePath`

Table-driven tests:
- Epic → `PROJ/PROJ-1 - Epic Name/_epic.md`
- Story with epic parent → `PROJ/PROJ-1 - Epic Name/PROJ-10 - Story.md`
- Orphan (no parent, not epic) → `PROJ/PROJ-50 - Orphan.md`
- Parent with nil Fields → `PROJ/PROJ-1 - /PROJ-10 - Story.md` (degrades to key-only dir)
- Nil IssueType → treated as non-epic, goes flat
- Epic with parent (epic-as-child-of-epic) → treated as top-level epic, not nested (flat at project root with its own folder)

### 4. `internal/cmd/issue/export_test.go` — Integration tests

New fixture `treeExportIssues()` returning: 1 Epic, 2 Stories (parent = epic), 1 orphan Bug.

Tests:
- `TestExportTree` — verify file paths match hierarchy, `_epic.md` exists
- `TestExportTreeDefault` — without `--tree`, same issues produce flat layout (no regression)

## Files to modify

| File | Action |
|------|--------|
| `internal/markdown/filemap.go` | Add `IssueTreePath()`, extract `issueProjectKey()` helper |
| `internal/cmd/issue/export.go` | Add `Tree` field + flag + conditional dispatch |
| `internal/markdown/filemap_test.go` | Add `TestIssueTreePath` |
| `internal/cmd/issue/export_test.go` | Add `treeExportIssues()`, `TestExportTree`, `TestExportTreeDefault` |

## Import compatibility

`ParseDir` uses `filepath.WalkDir` collecting all `.md` files recursively. `_epic.md` has frontmatter with `key:` field. No import changes needed — verified by reading `parse.go:93-122`.

## Verification

```bash
cd /Users/andrei/projects/odevo/mvp/jira-cli
go test ./internal/markdown/ -run TestIssueTreePath -v
go test ./internal/cmd/issue/ -run TestExportTree -v
go test ./internal/cmd/issue/ -run TestExportTreeDefault -v
go test ./... # full suite, no regressions
```
