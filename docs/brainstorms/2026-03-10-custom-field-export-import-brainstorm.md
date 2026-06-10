# Brainstorm: Custom Field Export/Import

**Date:** 2026-03-10
**Status:** Ready for planning

## What We're Building

Extend `jira issue export` and `jira issue import` to round-trip custom fields (e.g., Team, Story Points) alongside the existing built-in fields. Custom fields appear in YAML frontmatter by their human-readable display name, with automatic name-to-ID resolution via the Jira field metadata API.

### Motivation

Andrei uses a "Team" custom field on epics. Currently, export/import ignores all custom fields, so round-tripping loses this data. The goal is full-fidelity markdown files that include custom fields.

## Why This Approach

### Display names over field IDs

Users shouldn't need to know `customfield_10001`. The CLI resolves names to IDs by calling `GET /rest/api/3/field` on every export/import (one extra API call, no caching complexity). The `schema fields` command already uses this endpoint.

### All fields by default, filterable

Export includes all custom fields with values by default. A `--fields` flag allows limiting to specific fields by name (e.g., `--fields team,story_points`). This avoids requiring users to know their field names upfront while allowing targeted exports for large instances.

### String-only values on import

Import sends custom field values as plain strings, matching the existing `--field key=value` behavior. Jira coerces many types server-side (text, number, simple options). This avoids the complexity of type-aware coercion for now. If structured types (multi-select, cascading select) need proper support, that's a future enhancement.

### Built-in wins on collision

If a custom field name (lowercased, snake_cased) collides with a built-in frontmatter key (summary, status, etc.), the custom field is skipped during export with a warning to stderr. This prevents ambiguity without requiring namespacing.

## Key Decisions

1. **Frontmatter format:** Custom fields are top-level YAML keys by display name (lowercased, spaces to underscores). E.g., `team: Platform`, `story_points: 5`.

2. **Export scope:** All custom fields with non-null values by default. `--fields name1,name2` to filter.

3. **Import value handling:** String-only. No type coercion. Matches `--field` behavior.

4. **Name resolution:** Fetch `GET /rest/api/3/field` on every export/import. No caching.

5. **Collision strategy:** Built-in frontmatter keys win. Warning to stderr for shadowed custom fields.

6. **Field name normalization:** Display name lowercased, spaces replaced with underscores. E.g., "Story Points" -> `story_points`, "Team" -> `team`.

## Scope

### In scope

- Export: request all fields (or `--fields` subset), write custom fields to frontmatter by display name
- Import: read custom fields from frontmatter, resolve names to IDs, send as string values
- Field metadata fetch via existing `api.ListFields`
- Name normalization (lowercase, snake_case)
- Collision detection with built-in keys (warn + skip)

### Out of scope

- Type-aware coercion (option objects, multi-select arrays, user lookups)
- Field metadata caching
- Per-project/per-issuetype field filtering
- Custom field support in `create`/`edit` by name (they already have `--field` by ID)

## Open Questions

None — all decisions resolved.

## Technical Notes

- `api.IssueFields.CustomFields` (`map[string]json.RawMessage`) already captures all custom fields during JSON unmarshal
- `api.ListFields` already exists and returns `[]api.Field` with ID, Name, Schema, Custom bool
- `exportFields` list needs to accept `"*"` or be removed in favor of requesting all fields
- `Frontmatter` struct needs a `CustomFields map[string]interface{}` (or similar) with custom YAML marshal/unmarshal to flatten into top-level keys
- `markdown.ParseFile` needs to capture unknown frontmatter keys as custom fields
- `buildCreateFields` / `buildUpdateFields` need to pass through custom field entries
