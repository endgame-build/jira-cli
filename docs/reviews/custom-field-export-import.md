# PR Review: Custom Field Export/Import

**Branch:** `ralph/custom-field-export-import`
**Date:** 2026-03-12
**Files:** 8 changed, ~1450 lines added

## Critical Issues (1)

- **parse.go:75** — Second YAML unmarshal failure silently drops ALL custom fields. `if err == nil` guard means a failure skips custom field extraction entirely with no error returned. Direct violation of "no silent fallbacks" rule.

## High Issues (2)

- **frontmatter.go:130** — `extractCustomFieldValue` swallows JSON unmarshal errors, returns `(nil, false)` indistinguishable from "unsupported type". Misleading warning message misdirects debugging.
- **import.go:397-399** — `buildImportFieldMap` silently drops duplicate normalized field names with no warning. Export path warns about collisions; import path doesn't. Could map user's custom field to the wrong Jira field ID.

## Medium Issues (4)

- **import.go:411** — `injectCustomFields` silent `continue` relies on distant validation invariant. A refactor could break this silently.
- **export.go:234** — Unmatched `--fields` names produce only a warning, allowing silent data loss on typos.
- **frontmatter.go:93,120** — No bounds check on `node.Content[0]` / `valNode.Content[0]` — potential panic on unexpected YAML node structure.
- **frontmatter.go MarshalYAML** — Zero direct unit tests despite being the most fragile code (yaml.Node manipulation). Only tested indirectly via string matching on output.

## Suggestions (5)

- No export-to-import round-trip test. Type coercion mismatches (JSON float64 vs YAML int) could cause silent data corruption.
- Extract custom field processing from `IssueToMarkdown` (~40 lines) into its own function for readability/testability.
- Consolidate 4 sequential validation loops over `issueFiles` in `runImport` into fewer passes.
- Deduplicate switch logic in `extractCustomFieldValue` (`.value` and `.name` paths use identical type switches).
- `buildImportFieldMap` duplicate normalized name behavior and `--fields` edge cases (spaces, empty segments) untested.

## Strengths

- Code reviewer found no issues at high confidence — clean implementation following all project conventions.
- Command pattern, error types, factory DI, layer ordering all correct.
- Validation coverage thorough: missing key, missing project, temp-to-temp parent, unknown custom fields.
- Conflict detection well-tested (matching/mismatched timestamps, `--force`).
- `extractCustomFieldValue` has excellent table-driven test coverage.
- Dry-run tests verify no side effects in both export and import.

## Recommended Action

1. Fix the critical `parse.go:75` silent fallback — return error instead of `if err == nil` guard.
2. Address the 2 high issues — distinguish unmarshal errors from unsupported types; warn on duplicate normalized names in import.
3. Consider the medium issues — bounds checks, invariant panics, `--fields` strictness.
4. Add `MarshalYAML` unit tests and an export-to-import round-trip test.
