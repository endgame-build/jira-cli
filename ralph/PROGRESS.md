# Ralph Progress Log
Started: Wed Mar 11 16:42:43 +03 2026
---

## T-001 - Add NormalizeFieldName and built-in key set to frontmatter.go
Status: COMPLETED
Package: markdown

### Changes
- internal/markdown/frontmatter.go — added NormalizeFieldName, builtinFrontmatterKeys, IsBuiltinKey

### Success Criteria
- [x] SC 1 — NormalizeFieldName lowercases, replaces spaces, strips non-[a-z0-9_]
- [x] SC 2 — returns empty string for names that normalize to nothing
- [x] SC 3 — builtinFrontmatterKeys contains all 15 keys
- [x] SC 4 — IsBuiltinKey returns true/false correctly
- [x] SC 5 — build passes

### Learnings
**Patterns**: regex compiled at package level (`var nonAlphanumUnderscore = regexp.MustCompile(...)`)
---

## T-002 - Add CustomFields to Frontmatter and update IssueToMarkdown
Status: COMPLETED
Package: markdown

### Changes
- internal/markdown/frontmatter.go — added CustomFields field (yaml:"-"), MarshalYAML, extractCustomFieldValue, updated IssueToMarkdown signature
- internal/markdown/frontmatter_test.go — updated existing test call sites to pass nil, nil
- internal/cmd/issue/export.go — updated writeFileAtomic call site to pass nil, nil

### Success Criteria
- [x] SC 1 — Frontmatter.CustomFields is yaml:"-"
- [x] SC 2 — MarshalYAML appends custom fields after built-in fields (sorted)
- [x] SC 3 — IssueToMarkdown accepts fields map + io.Writer, nil/nil preserves old behavior
- [x] SC 4 — extractCustomFieldValue handles string, number, bool, null, object .value/.name, array
- [x] SC 5 — Built-in key collision: skipped + warning
- [x] SC 6 — Custom-to-custom collision: first wins + warning with both field IDs
- [x] SC 7 — export.go call site updated to pass nil
- [x] SC 8 — Build passes

### Learnings
**Patterns**: MarshalYAML with type alias to avoid recursion; return mapping node (not document node) from yaml.Unmarshal result — `node.Content[0]`
**Gotchas**: yaml.Unmarshal into yaml.Node produces a document node wrapping the actual content; returning the document node from MarshalYAML causes "expected SCALAR... but got document start" error
---

## T-003 - Add tests for NormalizeFieldName, extractCustomFieldValue, and custom field marshaling
Status: COMPLETED
Package: markdown

### Changes
- internal/markdown/frontmatter_test.go — added TestNormalizeFieldName, TestIsBuiltinKey, TestExtractCustomFieldValue, TestIssueToMarkdownWithCustomFields, TestIssueToMarkdownBuiltinCollision, TestIssueToMarkdownNilFields, TestIssueToMarkdownWarnWriter

### Success Criteria
- [x] SC 1 — NormalizeFieldName tested with 7 cases including empty result
- [x] SC 2 — IsBuiltinKey tested for built-in and custom keys
- [x] SC 3 — extractCustomFieldValue tested for all 9 value types (string, number, bool, null, object .value string, object .value number, object .name, object neither, array)
- [x] SC 4 — IssueToMarkdown with custom fields verified in YAML output
- [x] SC 5 — Built-in key collision verified (absent from output, warning in warnWriter)
- [x] SC 6 — nil/nil parameters preserve backward compatibility
- [x] SC 7 — nil warnWriter doesn't panic
- [x] SC 8 — Tests pass

### Learnings
**Patterns**: Same-package tests can directly call unexported functions like extractCustomFieldValue
---

## T-004 - Add custom field unmarshaling to parse.go for import
Status: COMPLETED
Package: markdown

### Changes
- internal/markdown/parse.go — added second-pass YAML unmarshal to capture unknown keys as CustomFields

### Success Criteria
- [x] SC 1 — ParseFile populates Frontmatter.CustomFields with unknown YAML keys
- [x] SC 2 — Built-in keys are NOT duplicated in CustomFields (IsBuiltinKey filter)
- [x] SC 3 — Existing ParseFile tests still pass (no regressions)
- [x] SC 4 — Build passes

### Learnings
**Patterns**: Second-pass unmarshal into map[string]interface{} is a clean way to capture unknown YAML keys without changing struct tags
---

## T-005 - Update export command to fetch field metadata and pass to IssueToMarkdown
Status: COMPLETED
Package: cmd/issue

### Changes
- internal/cmd/issue/export.go — added Fields to ExportOptions, --fields flag, buildExportFieldMap(), removed exportFields var, updated writeFileAtomic signature to accept field map + warnWriter
- internal/cmd/issue/export_test.go — updated exportSearchHandler to serve GET /field endpoint (empty array)

### Success Criteria
- [x] SC 1 — --fields flag registered with correct help text
- [x] SC 2 — ListFields called once per export run (before pagination loop) via buildExportFieldMap
- [x] SC 3 — Field map built from ListFields response, keyed by field ID
- [x] SC 4 — --fields filters custom fields by normalized name match
- [x] SC 5 — --fields with unknown name warns to stderr and continues
- [x] SC 6 — exportFields variable removed, search requests all fields (nil → defaults to *all)
- [x] SC 7 — writeFileAtomic passes field map + warnWriter to IssueToMarkdown
- [x] SC 8 — Build passes (make test, make lint, make build all green)

### Learnings
**Patterns**: SearchIssues defaults Fields to ["*all"] when nil, so removing the explicit field list is safe. buildExportFieldMap extracts the ListFields+filter logic into a testable helper.
---

## T-006 - Update import command to resolve custom fields to Jira field IDs
Status: COMPLETED
Package: cmd/issue

### Changes
- internal/cmd/issue/import.go — added buildImportFieldMap, injectCustomFields, collectCustomFieldNames; updated buildCreateFields/buildUpdateFields signatures to accept customFieldMap; added custom field validation loop and dry-run custom field display
- internal/cmd/issue/import_test.go — added GET /field endpoint to all test handlers (importHandler + 4 inline handlers)

### Success Criteria
- [x] SC 1 — ListFields called once per import run via buildImportFieldMap
- [x] SC 2 — Reverse lookup map built: normalizedName → fieldID in buildImportFieldMap
- [x] SC 3 — Unresolvable frontmatter keys error with VALIDATION_ERROR and suggestion
- [x] SC 4 — buildCreateFields injects custom fields by field ID via injectCustomFields
- [x] SC 5 — buildUpdateFields injects custom fields by field ID via injectCustomFields
- [x] SC 6 — Custom field values sent as-is (direct assignment from CustomFields map)
- [x] SC 7 — Build passes (make test, make lint, make build all green)

### Learnings
**Patterns**: buildImportFieldMap builds normalizedName→fieldID reverse lookup (mirrors buildExportFieldMap pattern but reversed). injectCustomFields is shared between create and update paths. Existing tests all need GET /field handler added when ListFields is called unconditionally.
---

## T-007 - Add tests for custom field parsing in parse.go
Status: COMPLETED
Package: markdown

### Changes
- internal/markdown/parse_test.go — added TestParseFileCustomFields, TestParseFileNoCustomFields, TestParseFileOnlyCustomFields, TestParseDirWithCustomFields

### Success Criteria
- [x] SC 1 — Custom fields parsed correctly alongside built-in fields (TestParseFileCustomFields, TestParseFileOnlyCustomFields)
- [x] SC 2 — Built-in keys not duplicated in CustomFields (TestParseFileCustomFields verifies absence)
- [x] SC 3 — Files with no custom fields have nil CustomFields (TestParseFileNoCustomFields)
- [x] SC 4 — ParseDir correctly parses custom fields from multiple files (TestParseDirWithCustomFields)
- [x] SC 5 — Tests pass (go test ./internal/markdown/... all green)

### Learnings
**Patterns**: YAML unmarshals integers as `int` (not `float64` like JSON), so type assertions in tests need `int` not `float64`.
---

## T-008 - Add tests for custom field export
Status: COMPLETED
Package: cmd/issue

### Changes
- internal/cmd/issue/export_test.go — added issueWithCustomFieldsJSON helper, customFieldTestFields, customFieldExportHandler, TestExportCustomFields, TestExportFieldsFlag, TestExportFieldsFlagUnknown, TestExportBuiltinCollision, TestExportCustomFieldObjectValue, TestExportCustomFieldSkipArray

### Success Criteria
- [x] SC 1 — Custom field export with scalar values verified (TestExportCustomFields)
- [x] SC 2 — --fields filtering works correctly (TestExportFieldsFlag)
- [x] SC 3 — --fields with unknown name produces warning (TestExportFieldsFlagUnknown)
- [x] SC 4 — Built-in key collision skips custom field (TestExportBuiltinCollision)
- [x] SC 5 — Object with .value extracted correctly (TestExportCustomFieldObjectValue)
- [x] SC 6 — Array values skipped with warning (TestExportCustomFieldSkipArray)
- [x] SC 7 — Tests pass (go test ./internal/cmd/issue/... all green, make test all green)

### Learnings
**Patterns**: IssueFields.CustomFields is `json:"-"`, so mock servers can't use `json.Encoder` with Go structs to include custom fields. Must build raw JSON with custom field keys injected at the fields level (issueWithCustomFieldsJSON helper). The client's custom UnmarshalJSON then picks them up.
---

## T-009 - Add tests for custom field import
Status: COMPLETED
Package: cmd/issue

### Changes
- internal/cmd/issue/import_test.go — added customFieldTestFieldsImport, customFieldImportHandler, TestImportCustomFields, TestImportCustomFieldUpdate, TestImportUnresolvableKey, TestImportDryRunCustomFields, TestImportNoCustomFields

### Success Criteria
- [x] SC 1 — Create with custom fields sends correct field IDs in API payload (TestImportCustomFields)
- [x] SC 2 — Update with custom fields sends correct field IDs (TestImportCustomFieldUpdate)
- [x] SC 3 — Unresolvable key returns VALIDATION_ERROR with helpful message (TestImportUnresolvableKey)
- [x] SC 4 — --dry-run with custom fields validates but makes no API calls (TestImportDryRunCustomFields)
- [x] SC 5 — Import without custom fields works (TestImportNoCustomFields)
- [x] SC 6 — Tests pass (go test ./internal/cmd/issue/... all green, make test all green)

### Learnings
**Patterns**: For import tests, custom fields come through YAML frontmatter parsing (not JSON like export), so YAML integer values (5) become int in Go, then float64 after JSON round-trip through the captured request body. The customFieldImportHandler captures both create and edit bodies via separate pointers.
---
