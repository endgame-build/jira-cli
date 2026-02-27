// Package output provides formatters for CLI output: text tables, JSON envelopes,
// jq filtering, and error rendering. All commands use this package to produce output.
package output

import (
	"encoding/json"
	"io"

	"github.com/endgame-build/jira-cli/internal/api"
)

// PaginationMeta describes the pagination state returned in list JSON envelopes.
// Re-exported from api package to avoid breaking callers.
type PaginationMeta = api.PaginationMeta

// listEnvelope wraps list data with pagination metadata.
// JSON shape: {"data": [...], "pagination": {...}}
type listEnvelope struct {
	Data       interface{}         `json:"data"`
	Pagination *api.PaginationMeta `json:"pagination"`
}

// mutationEnvelope wraps mutation results with ok:true.
// JSON shape: {"ok": true, ...extraFields}
type mutationEnvelope struct {
	OK     bool `json:"ok"`
	extras map[string]interface{}
}

// MarshalJSON flattens ok + extras into one object: {"ok":true, "key":"PROJ-1", ...}.
func (m mutationEnvelope) MarshalJSON() ([]byte, error) {
	out := make(map[string]interface{}, len(m.extras)+1)
	out["ok"] = m.OK
	for k, v := range m.extras {
		out[k] = v
	}
	return json.Marshal(out)
}

// writeJSON encodes v as indented JSON followed by a newline.
func writeJSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// writeDataJSON writes a bare JSON object (for single-item data output).
func writeDataJSON(w io.Writer, data interface{}) error {
	return writeJSON(w, data)
}

// writeListJSON writes the list envelope: {"data": [...], "pagination": {...}}.
func writeListJSON(w io.Writer, items interface{}, pagination *PaginationMeta) error {
	return writeJSON(w, listEnvelope{
		Data:       items,
		Pagination: pagination,
	})
}

// writeMutationJSON writes {"ok": true, ...extras}.
func writeMutationJSON(w io.Writer, extras map[string]interface{}) error {
	return writeJSON(w, mutationEnvelope{OK: true, extras: extras})
}
