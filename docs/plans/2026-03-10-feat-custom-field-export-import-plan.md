---
title: "feat: Custom field export/import"
type: feat
status: active
date: 2026-03-10
origin: docs/brainstorms/2026-03-10-custom-field-export-import-brainstorm.md
---

# feat: Custom field export/import

## Overview

Extend `jira issue export` and `jira issue import` to round-trip custom fields (e.g., Team, Story Points) alongside built-in fields. Custom fields appear in YAML frontmatter by human-readable display name with automatic name-to-ID resolution via the Jira field metadata API.

## Problem Statement / Motivation

Export/import currently ignores all custom fields. Round-tripping loses data like "Team" on epics. Users need full-fidelity markdown files that preserve custom field values.

(see brainstorm: docs/brainstorms/2026-03-10-custom-field-export-import-brainstorm.md)

## Proposed Solution

### Export

1. Fetch field metadata via `GET /rest/api/3/field` (existing `api.ListFields`)
2. Request all fields from Jira search (remove hardcoded `exportFields` list)
3. For each issue, extract custom field values from `IssueFields.CustomFields` (`map[string]json.RawMessage`)
4. Normalize field display names to YAML keys (lowercase, spaces → underscores)
5. Write scalar values to frontmatter; skip complex types (arrays, nested objects) with warning
6. `--fields name1,name2` filters which custom fields appear (all by default)

### Import

1. Parse frontmatter, capturing unknown keys as custom fields (two-pass YAML unmarshal)
2. Fetch field metadata to resolve normalized names back to field IDs
3. Inject custom field values (as strings) into the `fields` map sent to Jira
4. Error and abort if any frontmatter key can't be resolved to a Jira field

### Value Serialization Rules (Export)

| `json.RawMessage` type | YAML output | Example |
|---|---|---|
| String | Scalar | `team: Platform` |
| Number | Scalar | `story_points: 5` |
| Boolean | Scalar | `flagged: true` |
| Null | Omitted | (not written) |
| Object with `.value` key | Extract `.value` (preferred) | `severity: Critical` |
| Object with `.name` key (no `.value`) | Extract `.name` (fallback) | `component: Backend` |
| Object (no `.value` or `.name`) | Skip + warn stderr | — |
| Array | Skip + warn stderr | — |

**Extraction precedence for objects:** Try `.value` first, then `.name`. If neither exists or the extracted value is not a string/number, skip + warn.

### Name Normalization

Single shared function `NormalizeFieldName(displayName string) string`:
- `strings.ToLower`
- Replace spaces with underscores
- Strip all characters not in `[a-z0-9_]`
- If result is empty after stripping (e.g., field named `"!!!"`), skip + warn
- Used identically on export (name → key) and import (metadata name → key for matching)

### Collision Handling

- **Custom vs built-in:** Built-in frontmatter keys win. Skip custom field + warn to stderr. Built-in keys: `key`, `id`, `type`, `summary`, `status`, `priority`, `labels`, `parent`, `assignee`, `assignee_id`, `reporter`, `reporter_id`, `project`, `created`, `updated`.
- **Custom vs custom:** If two fields normalize to the same key, keep the first, warn about the collision with both field IDs.

## Technical Considerations

### Files to modify

| File | Changes |
|---|---|
| `internal/markdown/frontmatter.go` | Add `CustomFields map[string]interface{}` to `Frontmatter`. Custom `MarshalYAML`/`UnmarshalYAML`. Add `NormalizeFieldName`. Update `IssueToMarkdown` to extract and write custom fields. |
| `internal/markdown/parse.go` | Two-pass unmarshal in `ParseFile`: struct + raw map, diff keys → custom fields. |
| `internal/cmd/issue/export.go` | Remove `exportFields` or pass empty (fetch all fields). Add `--fields` flag. Fetch field metadata via `ListFields`. Build name→ID map, pass to `IssueToMarkdown`. |
| `internal/cmd/issue/import.go` | Fetch field metadata. Resolve custom frontmatter keys to field IDs. Inject into `buildCreateFields`/`buildUpdateFields`. Error on unresolvable keys. |
| `internal/markdown/frontmatter_test.go` | Tests for `NormalizeFieldName`, custom marshal/unmarshal, `IssueToMarkdown` with custom fields. |
| `internal/markdown/parse_test.go` | Tests for parsing frontmatter with unknown keys. |
| `internal/cmd/issue/export_test.go` | Tests for `--fields` flag, custom field export, collision warnings. |
| `internal/cmd/issue/import_test.go` | Tests for custom field import, name resolution, unresolvable key errors. |

### Architecture impacts

- `IssueToMarkdown` signature changes to `IssueToMarkdown(issue api.Issue, fields map[string]api.Field) (string, error)` where the map is keyed by field ID (e.g., `"customfield_10001"`). Each `api.Field` has `.Name` (for normalization) and `.Schema` (for type-aware extraction). Passing `nil` preserves current behavior (no custom fields).
- `Frontmatter` gains custom YAML marshal/unmarshal, similar pattern to `IssueFields.UnmarshalJSON` (two-pass decode, diff known keys)
- Export fetches field metadata once per run (before the pagination loop), not per-issue

### Performance implications

- One extra API call per export/import run (`GET /rest/api/3/field`)
- Requesting all fields increases response payload (~2-5x per issue). Acceptable for CLI batch operations. No `--fields`-based API optimization in v1.

### Security considerations

- Custom field values are user data from Jira — already trusted in the existing flow
- No new credential or auth surface

## Acceptance Criteria

- [ ] `jira issue export --project PROJ` includes custom fields with non-null scalar values in frontmatter by normalized display name
- [ ] Option-type custom fields extract `.value` or `.name` to a scalar
- [ ] Complex custom fields (arrays, nested objects without `.value`/`.name`) are skipped with warning to stderr
- [ ] `--fields team,story_points` limits exported custom fields to the named subset
- [ ] `--fields` accepts normalized names, matching is case-insensitive
- [ ] `--fields` with a non-existent field name warns to stderr and continues
- [ ] Built-in frontmatter key collisions: custom field skipped + warning
- [ ] Custom-to-custom name collisions: first wins + warning with both field IDs
- [ ] `jira issue import ./PROJ/` reads custom fields from frontmatter, resolves names to field IDs, sends as string values
- [ ] Import errors and aborts if a frontmatter key is not a known built-in key AND can't be resolved to a Jira custom field
- [ ] Null custom fields are omitted from frontmatter (not written as `null`)
- [ ] `--dry-run` on export/import shows custom fields in the preview
- [ ] Round-trip preserves scalar custom field values (string, number, boolean)
- [ ] All new code has table-driven tests
- [ ] `NormalizeFieldName` is a single shared function used by both export and import

## Success Metrics

- Custom fields like "Team" round-trip through export → edit → import without data loss
- No regressions in existing export/import tests

## Dependencies & Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Jira rejects string values for typed fields (e.g., number sent as `"5"`) | Medium | Document supported types. Jira coerces most scalars. Type-aware coercion is future scope. |
| Field metadata API unavailable (403, rate limit) | Low | Fail with clear error. No graceful degradation — custom fields require metadata. |
| Large instances with many custom fields slow down export | Low | Response size increases but is bounded. `--fields` reduces frontmatter bloat. |
| User adds arbitrary keys to frontmatter (notes, comments) | Medium | Import errors on unresolvable keys. Users can workaround by prefixing with `#` (YAML comments) or using the markdown body. |

## Sources & References

- **Origin brainstorm:** [docs/brainstorms/2026-03-10-custom-field-export-import-brainstorm.md](docs/brainstorms/2026-03-10-custom-field-export-import-brainstorm.md) — Key decisions: display names over IDs, all-by-default with `--fields` filter, string-only import, no caching, built-in wins on collision.
- Existing field metadata: `internal/api/schema.go:11` (`ListFields`)
- Custom field capture: `internal/api/types.go:378` (`IssueFields.UnmarshalJSON`)
- Export entry: `internal/cmd/issue/export.go:61` (`runExport`)
- Import field building: `internal/cmd/issue/import.go:285` (`buildCreateFields`)
- Frontmatter struct: `internal/markdown/frontmatter.go:15`
- Parse logic: `internal/markdown/parse.go:37` (`ParseFile`)
