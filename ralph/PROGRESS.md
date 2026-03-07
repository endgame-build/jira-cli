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
