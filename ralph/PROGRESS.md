# Ralph Progress Log
Started: Sat Mar  8 2026
---

## Codebase Patterns

- **Test helpers**: `factory.NewTestFactory(ios, cfg, client)` for pre-wired factory; `iostreams.Test()` for stdout/stderr buffers; `httptest.NewServer` for API mocks; `keyring.MockInit()` for in-memory keyring
- **Config test isolation**: `t.TempDir()` for config files, `t.Setenv()` for env vars
- **Table-driven tests**: standard Go pattern used throughout the codebase
- **Command pattern**: `NewCmdXxx(f *factory.Factory) *cobra.Command` + `XxxOptions` + `runXxx`
- **Error handling**: all errors are `CLIError` with code, message, context, suggestion
- **Output shapes**: `OutputData` (single), `OutputList` (paginated), `OutputMutation` (`"ok": true`), `OutputDryRun`, `OutputDryRunWithContext`
- **Browser test pattern**: `BrowserOpen` on options structs — override in tests to capture URLs
- **Layer order**: errors → iostreams → config → auth → api → output → adf → shared → factory → commands → main (no upward imports)

---

## T-001 - Add gopkg.in/yaml.v3 dependency
Status: COMPLETED
Package: deps

### Changes
- go.mod — added `gopkg.in/yaml.v3 v3.0.1` (indirect until first import)
- go.sum — updated with yaml.v3 checksums

### Success Criteria
- [x] SC 1 — go.mod contains gopkg.in/yaml.v3, verified by grep
- [x] SC 2 — go.sum updated, verified by grep
- [x] SC 3 — `go build ./...` passes

### Learnings
**Gotchas**: `go mod tidy` removes deps that aren't imported yet. Used `go get` without `go mod tidy` to keep the dependency in go.mod as indirect until T-004 creates the first import.
---

## T-002 - Add adf.ToMarkdown() — ADF-to-Markdown converter
Status: COMPLETED
Package: adf

### Changes
- internal/adf/to_markdown.go — added ToMarkdown() function with full ADF node type and mark support

### Success Criteria
- [x] SC 1 — ToMarkdown correctly converts each ADF node type to its Markdown equivalent
- [x] SC 2 — Unknown node types fall back to ExtractText behavior (concatenate child text)
- [x] SC 3 — Empty/nil input returns empty string
- [x] SC 4 — Invalid JSON returns raw string as fallback
- [x] SC 5 — Typecheck passes, verified by go vet ./...

### Learnings
**Patterns**: Follows same entry pattern as `ToPlaintext` (unmarshal + recursive render). Key difference: marks produce real Markdown syntax (`**bold**`, `*italic*`, etc.) instead of being stripped. Used separate `mdRender*` prefix to avoid name collisions with existing `render*` functions.
**Gotchas**: JSON unmarshals numeric attrs as `float64`, so heading level needs a type switch (`float64` from JSON, `int` from programmatic construction).
---

## T-003 - Add adf.ToMarkdown tests including round-trip
Status: COMPLETED
Package: adf

### Changes
- internal/adf/to_markdown_test.go — 30+ table-driven tests for ToMarkdown + 10 round-trip tests

### Success Criteria
- [x] SC 1 — All node types have individual test cases (heading×4, paragraph×2, bullet/ordered/nested lists×5, code block×3, blockquote×2, rule, hard break, each mark type, combined marks, complex doc)
- [x] SC 2 — Round-trip test verifies ADF→MD→ADF for 10 supported node/mark types
- [x] SC 3 — Edge cases (nil, empty, invalid JSON, empty doc, empty paragraph) covered
- [x] SC 4 — Unknown node type fallback tested (table→text, media→empty)
- [x] SC 5 — Tests pass (go test + go vet clean)

### Learnings
**Patterns**: Reuses `mustMarshal` helper from `plaintext_test.go` (same package). Unknown node tests use inline IIFE to build custom node types not in the constructor set.
**Gotchas**: Mark application order matters — `Strong(), Code()` wraps as `` `**text**` `` (code outermost), not `**`text`**`. Ordered list indent is 3 spaces per level (matches `mdRenderOrderedItem` using `strings.Repeat("   ", depth)`), not 2.
---

## T-004 - Create internal/markdown package — frontmatter.go and filemap.go
Status: COMPLETED
Package: markdown

### Changes
- internal/markdown/frontmatter.go — Frontmatter struct with YAML tags, IssueToMarkdown() converter
- internal/markdown/filemap.go — IssuePath() for file paths, SanitizeFilename() for safe names

### Success Criteria
- [x] SC 1 — Frontmatter struct has all fields with correct YAML tags
- [x] SC 2 — IssueToMarkdown produces correct YAML frontmatter + markdown body
- [x] SC 3 — IssueToMarkdown nil-checks all 7 pointer fields before access
- [x] SC 4 — IssuePath returns correct relative path format (ProjectKey/IssueKey - Summary.md)
- [x] SC 5 — SanitizeFilename handles special chars, space collapsing, 100-char limit
- [x] SC 6 — Typecheck passes (go vet + go build clean)

### Learnings
**Patterns**: `internal/markdown` sits between `api` and `shared` in layer hierarchy — imports `api` and `adf` but NOT `factory` or commands. Uses `gopkg.in/yaml.v3` for frontmatter marshaling. yaml.v3 was added as indirect in T-001, now promoted to direct import.
**Gotchas**: `api.IssueFields` has 7 pointer fields that need nil-checks: IssueType, Status, Priority, Project, Parent, Assignee, Reporter. IssuePath falls back to extracting project key from issue key prefix (e.g. "PROJ-123" → "PROJ") when Project is nil.
---

## T-005 - Create internal/markdown/parse.go — file parsing for import
Status: COMPLETED
Package: markdown

### Changes
- internal/markdown/parse.go — added IssueFile struct, IsCreate(), ParseFile(), ParseDir()

### Success Criteria
- [x] SC 1 — ParseFile splits frontmatter from body via delimiter parsing
- [x] SC 2 — IsCreate detects temp key pattern via regexp `^[A-Z]+-NEW-\d+$`
- [x] SC 3 — ParseDir recursively walks .md files sorted by path
- [x] SC 4 — Validation errors return CLIError with VALIDATION_ERROR code
- [x] SC 5 — Missing key returns error; empty body allowed
- [x] SC 6 — Typecheck passes (go vet + go build clean)

### Learnings
**Patterns**: ParseFile uses string-based frontmatter splitting (find `---\n` delimiters). Handles edge case where closing `---` is at EOF without trailing newline. Uses `clierrors` import alias for `internal/errors` to avoid collision with stdlib `errors`.
**Gotchas**: Frontmatter body offset calculation: `4 (opening ---\n) + closeIdx + 4 (closing \n---\n)`. The closing delimiter search starts at offset 4 (after opening delimiter).
---

## T-006 - Add tests for internal/markdown package
Status: COMPLETED
Package: markdown

### Changes
- internal/markdown/frontmatter_test.go — IssueToMarkdown tests (full struct, nil fields, structure)
- internal/markdown/filemap_test.go — IssuePath and SanitizeFilename tests (special chars, truncation, unicode)
- internal/markdown/parse_test.go — ParseFile, IsCreate, ParseDir tests (valid/invalid/edge cases, nested dirs)

### Success Criteria
- [x] SC 1 — IssueToMarkdown tested with full Issue struct and nil optional fields
- [x] SC 2 — IssuePath tested with special characters in summary
- [x] SC 3 — SanitizeFilename edge cases all covered (17 cases + truncation)
- [x] SC 4 — ParseFile tested for valid, invalid, and edge cases (7 cases + field check + not found)
- [x] SC 5 — IsCreate correctly distinguishes temp vs real keys (9 cases)
- [x] SC 6 — ParseDir tested with nested directories (3 test functions)
- [x] SC 7 — Tests pass (28 tests, go vet + go build clean)

### Learnings
**Patterns**: Used `writeTestFile` helper for creating temp markdown files in `t.TempDir()`. Used `errors.As` with `*clierrors.CLIError` for asserting error codes. `repeat` helper for building strings of exact length (avoid `make([]byte, n)` which creates null bytes).
**Gotchas**: YAML v3 marshaler doesn't always quote timestamps — test expectations must match actual yaml.Marshal output format. `SanitizeFilename` uses `len()` which counts bytes not runes, so unicode truncation may cut mid-character.
---

## T-007 - Create jira issue export command
Status: COMPLETED
Package: cmd/issue

### Changes
- internal/cmd/issue/export.go — added ExportOptions, NewCmdExport, runExport, buildExportJQL, writeFileAtomic

### Success Criteria
- [x] SC 1 — Export command registered with correct flags and help text
- [x] SC 2 — JQL built correctly from --project or --jql
- [x] SC 3 — Error if neither --jql nor --project/config-default provided (via resolveProject)
- [x] SC 4 — Pagination works via SearchResults.NextPageToken/IsLast
- [x] SC 5 — Files written with correct directory structure (PROJECT/KEY - Summary.md)
- [x] SC 6 — Temp-then-rename write pattern used
- [x] SC 7 — --dry-run collects paths without writing
- [x] SC 8 — --limit stops export at specified count
- [x] SC 9 — Progress reported to stderr
- [x] SC 10 — JSON output uses OutputMutation format
- [x] SC 11 — Typecheck passes, verified by go vet + go build

### Learnings
**Patterns**: Export uses manual pagination loop (not FetchTokenPage) because it needs streaming iteration over ALL pages rather than offset+limit semantics. Reuses `resolveProject` and `configGet` from create.go (same package).
**Gotchas**: pageSize must be capped to remaining limit to avoid fetching more than requested. Progress reporting every 50 issues keeps stderr useful without being noisy.
---

## T-008 - Add tests for jira issue export command
Status: COMPLETED
Package: cmd/issue

### Changes
- internal/cmd/issue/export_test.go — 12 tests covering all export functionality

### Success Criteria
- [x] SC 1 — Basic export writes correct files to temp directory (TestExportBasic)
- [x] SC 2 — File content matches mock issue data (TestExportFileContent)
- [x] SC 3 — --dry-run prevents file writes (TestExportDryRun, TestExportDryRunJSON)
- [x] SC 4 — --limit stops at specified count (TestExportLimit)
- [x] SC 5 — --json output matches OutputMutation format (TestExportJSON)
- [x] SC 6 — --jql bypasses project resolution (TestExportJQL)
- [x] SC 7 — No --jql and no --project/config → validation error (TestExportNoProject)
- [x] SC 8 — Pagination across multiple pages works (TestExportPagination)
- [x] SC 9 — Empty results handled gracefully (TestExportEmpty)
- [x] SC 10 — Tests pass (12 tests, all passing)

### Learnings
**Patterns**: Reused `newTestCreateFactory` helper from create_test.go (same package). Created `exportSearchHandler` that supports multi-page responses via a `pages [][]api.Issue` parameter — handler tracks `callCount` internally to serve different pages. Also reused `searchRequest` struct from list_test.go for decoding POST /search/jql.
**Gotchas**: The `exportIssues()` helper provides fully populated `api.Issue` structs including `json.RawMessage` for ADF description — this ensures `TestExportFileContent` can verify the ADF→Markdown conversion in the exported file.
---

## T-009 - Create jira issue import command
Status: COMPLETED
Package: cmd/issue

### Changes
- internal/cmd/issue/import.go — added ImportOptions, NewCmdImport, runImport, buildCreateFields, buildUpdateFields
- internal/markdown/parse.go — added IsTempKey() exported function for temp key detection

### Success Criteria
- [x] SC 1 — Import command registered with correct flags and help text
- [x] SC 2 — Positional args and --dir are mutually exclusive
- [x] SC 3 — No args and no --dir returns validation error
- [x] SC 4 — Creates require project and type in frontmatter, error if missing
- [x] SC 5 — Creates build correct CreateIssueInput — description set as *adf.Node directly
- [x] SC 6 — assignee_id maps to {accountId: ...}, missing assignee_id skips field
- [x] SC 7 — Temp-to-temp parent references rejected with VALIDATION_ERROR
- [x] SC 8 — Updates do NOT send type, project, parent, or status fields
- [x] SC 9 — Updates fetch current issue and perform conflict check
- [x] SC 10 — --force overrides conflict detection
- [x] SC 11 — Stop-on-first-error behavior
- [x] SC 12 — JSON output includes results array with action/key/temp_key/url and browse URLs
- [x] SC 13 — Typecheck passes, verified by go vet + go build

### Learnings
**Patterns**: Import follows the same factory DI pattern as create/edit. Two code paths (create vs update) separated cleanly by `IsCreate()`. `buildCreateFields` and `buildUpdateFields` extract field map construction, mirroring the shapes from create.go. Added `IsTempKey()` to markdown package as an exported helper for temp key detection (used for parent validation).
**Gotchas**: Conflict check compares `Frontmatter.Updated` vs `current.Fields.Updated` as plain strings — both sides must be non-empty for the check to trigger, so newly created issues (empty updated) skip conflict detection naturally.
---

## T-010 - Add tests for jira issue import command
Status: COMPLETED
Package: cmd/issue

### Changes
- internal/cmd/issue/import_test.go — 18 tests covering all import functionality

### Success Criteria
- [x] SC 1 — Create test verifies correct API payload (TestImportCreate: ADF *adf.Node in fields map)
- [x] SC 2 — assignee_id mapping tested (TestImportCreateWithAssignee + TestImportCreateWithoutAssignee)
- [x] SC 3 — Temp-to-temp parent reference rejection tested (TestImportTempToTempParent)
- [x] SC 4 — Missing project/type on create returns validation error (TestImportCreateMissingProject + TestImportCreateMissingType)
- [x] SC 5 — No args and no --dir returns validation error (TestImportNoArgsNoDir)
- [x] SC 6 — Conflict detection tested (TestImportConflictMismatch, TestImportConflictMatching, TestImportConflictEmptyTimestamp)
- [x] SC 7 — --force override tested (TestImportForceOverride)
- [x] SC 8 — --dry-run prevents API calls (TestImportDryRun)
- [x] SC 9 — Stop-on-first-error tested (TestImportStopOnFirstError)
- [x] SC 10 — Mixed create + update scenario works (TestImportMixed with JSON output)
- [x] SC 11 — Tests pass (18/18 passing)

### Learnings
**Patterns**: Created `newTestImportFactory` (simpler than `newTestCreateFactory` — no config needed) and `importHandler` with configurable `getIssueUpdated` parameter for conflict tests. Used `writeImportFile` helper for creating temp markdown files. Tested via both `cmd.Execute()` (for arg validation) and `runImport()` (for logic tests).
**Gotchas**: Cobra's `cmd.Execute()` prints error + usage to stderr, so validation tests through `cmd.Execute()` should only check error code, not stderr content. The `importHandler` GET endpoint strips `/issue/` prefix to extract key for dynamic responses.
---
