package output

import (
	"io"

	"github.com/jedib0t/go-pretty/v6/table"
)

// NewTable creates a go-pretty table writer with a clean, borderless style
// suitable for CLI output. The table writes to w.
func NewTable(w io.Writer) table.Writer {
	t := table.NewWriter()
	t.SetOutputMirror(w)
	t.SetStyle(table.StyleLight)
	t.Style().Options.DrawBorder = false
	t.Style().Options.SeparateHeader = false
	t.Style().Options.SeparateRows = false
	t.Style().Options.SeparateColumns = true
	return t
}

// TableFunc is a callback that populates a table writer for rendering.
// Commands implement this to define their text output layout.
type TableFunc func(t table.Writer)

// renderTable creates a table, applies the callback, and renders it.
func renderTable(w io.Writer, fn TableFunc) {
	t := NewTable(w)
	fn(t)
	t.Render()
}
