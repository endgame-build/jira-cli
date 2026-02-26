package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	clierrors "github.com/endgameio/jira-cli/internal/errors"
	"github.com/endgameio/jira-cli/internal/iostreams"
	"github.com/jedib0t/go-pretty/v6/table"
)

// --- JSON envelope tests ---

func TestWriteDataJSON(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]interface{}{"key": "PROJ-1", "summary": "Test issue"}
	if err := writeDataJSON(&buf, data); err != nil {
		t.Fatal(err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if got["key"] != "PROJ-1" {
		t.Errorf("key = %v, want PROJ-1", got["key"])
	}
	if got["summary"] != "Test issue" {
		t.Errorf("summary = %v, want Test issue", got["summary"])
	}
}

func TestWriteListJSON(t *testing.T) {
	var buf bytes.Buffer
	items := []map[string]string{{"key": "PROJ-1"}, {"key": "PROJ-2"}}
	total := 42
	pagination := &PaginationMeta{Offset: 0, Limit: 25, Total: &total, HasNextPage: true}

	if err := writeListJSON(&buf, items, pagination); err != nil {
		t.Fatal(err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Must have data and pagination keys.
	if _, ok := got["data"]; !ok {
		t.Fatal("missing 'data' key in list envelope")
	}
	if _, ok := got["pagination"]; !ok {
		t.Fatal("missing 'pagination' key in list envelope")
	}

	var dataArr []map[string]string
	if err := json.Unmarshal(got["data"], &dataArr); err != nil {
		t.Fatal(err)
	}
	if len(dataArr) != 2 {
		t.Errorf("data length = %d, want 2", len(dataArr))
	}

	var pg PaginationMeta
	if err := json.Unmarshal(got["pagination"], &pg); err != nil {
		t.Fatal(err)
	}
	if pg.Offset != 0 || pg.Limit != 25 || *pg.Total != 42 || !pg.HasNextPage {
		t.Errorf("pagination = %+v", pg)
	}
}

func TestWriteListJSON_NullTotal(t *testing.T) {
	var buf bytes.Buffer
	pagination := &PaginationMeta{Offset: 0, Limit: 25, Total: nil, HasNextPage: false}

	if err := writeListJSON(&buf, []string{}, pagination); err != nil {
		t.Fatal(err)
	}

	// Total should be null in JSON (token-based search).
	if !strings.Contains(buf.String(), `"total": null`) {
		t.Errorf("expected null total, got: %s", buf.String())
	}
}

func TestWriteMutationJSON(t *testing.T) {
	var buf bytes.Buffer
	extras := map[string]interface{}{
		"key":    "PROJ-1",
		"action": "created",
	}
	if err := writeMutationJSON(&buf, extras); err != nil {
		t.Fatal(err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if got["ok"] != true {
		t.Errorf("ok = %v, want true", got["ok"])
	}
	if got["key"] != "PROJ-1" {
		t.Errorf("key = %v, want PROJ-1", got["key"])
	}
	if got["action"] != "created" {
		t.Errorf("action = %v, want created", got["action"])
	}
}

func TestWriteMutationJSON_EmptyExtras(t *testing.T) {
	var buf bytes.Buffer
	if err := writeMutationJSON(&buf, nil); err != nil {
		t.Fatal(err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["ok"] != true {
		t.Errorf("ok = %v, want true", got["ok"])
	}
	// Only "ok" key expected.
	if len(got) != 1 {
		t.Errorf("expected 1 key, got %d: %v", len(got), got)
	}
}

// --- JQ tests ---

func TestApplyJQ_FieldExtract(t *testing.T) {
	var buf bytes.Buffer
	input := `{"key": "PROJ-1", "summary": "Test"}`
	if err := ApplyJQ(&buf, []byte(input), ".key"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "PROJ-1" {
		t.Errorf("jq .key = %q, want %q", got, "PROJ-1")
	}
}

func TestApplyJQ_ArrayIndex(t *testing.T) {
	var buf bytes.Buffer
	input := `{"data": [{"k": "A"}, {"k": "B"}]}`
	if err := ApplyJQ(&buf, []byte(input), ".data[0].k"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "A" {
		t.Errorf("jq .data[0].k = %q, want %q", got, "A")
	}
}

func TestApplyJQ_NumberOutput(t *testing.T) {
	var buf bytes.Buffer
	input := `{"count": 42}`
	if err := ApplyJQ(&buf, []byte(input), ".count"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "42" {
		t.Errorf("jq .count = %q, want %q", got, "42")
	}
}

func TestApplyJQ_NullOutput(t *testing.T) {
	var buf bytes.Buffer
	input := `{"x": null}`
	if err := ApplyJQ(&buf, []byte(input), ".x"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "null" {
		t.Errorf("jq .x = %q, want %q", got, "null")
	}
}

func TestApplyJQ_BoolOutput(t *testing.T) {
	var buf bytes.Buffer
	input := `{"ok": true}`
	if err := ApplyJQ(&buf, []byte(input), ".ok"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "true" {
		t.Errorf("jq .ok = %q, want %q", got, "true")
	}
}

func TestApplyJQ_MultipleResults(t *testing.T) {
	var buf bytes.Buffer
	input := `{"data": [{"k": "A"}, {"k": "B"}, {"k": "C"}]}`
	if err := ApplyJQ(&buf, []byte(input), ".data[].k"); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(buf.String())
	want := "A\nB\nC"
	if got != want {
		t.Errorf("jq .data[].k = %q, want %q", got, want)
	}
}

func TestApplyJQ_InvalidExpr(t *testing.T) {
	var buf bytes.Buffer
	err := ApplyJQ(&buf, []byte(`{}`), ".[invalid")
	if err == nil {
		t.Fatal("expected error for invalid jq expression")
	}
	if !strings.Contains(err.Error(), "invalid jq expression") {
		t.Errorf("error = %q, want 'invalid jq expression' prefix", err.Error())
	}
}

func TestApplyJQ_InvalidJSON(t *testing.T) {
	var buf bytes.Buffer
	err := ApplyJQ(&buf, []byte(`not json`), ".key")
	if err == nil {
		t.Fatal("expected error for invalid JSON input")
	}
	if !strings.Contains(err.Error(), "invalid JSON input") {
		t.Errorf("error = %q, want 'invalid JSON input' prefix", err.Error())
	}
}

func TestApplyJQ_EmptyResult(t *testing.T) {
	var buf bytes.Buffer
	input := `{"key": "val"}`
	// select(false) produces no output.
	if err := ApplyJQ(&buf, []byte(input), "select(false)"); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %q", buf.String())
	}
}

// --- Table tests ---

func TestNewTable(t *testing.T) {
	var buf bytes.Buffer
	tw := NewTable(&buf)
	tw.AppendHeader(table.Row{"Key", "Summary"})
	tw.AppendRow(table.Row{"PROJ-1", "First issue"})
	tw.AppendRow(table.Row{"PROJ-2", "Second issue"})
	tw.Render()

	out := buf.String()
	if !strings.Contains(out, "PROJ-1") {
		t.Errorf("table missing PROJ-1: %s", out)
	}
	if !strings.Contains(out, "PROJ-2") {
		t.Errorf("table missing PROJ-2: %s", out)
	}
	if !strings.Contains(out, "First issue") {
		t.Errorf("table missing 'First issue': %s", out)
	}
}

func TestNewTable_NoBorder(t *testing.T) {
	var buf bytes.Buffer
	tw := NewTable(&buf)
	tw.AppendRow(table.Row{"A", "B"})
	tw.Render()

	out := buf.String()
	// Light style with no border should not have a +--- border line.
	if strings.Contains(out, "+---") {
		t.Errorf("table has unexpected border: %s", out)
	}
}

// --- Error output tests ---

func TestOutputError_Text_CLIError(t *testing.T) {
	var buf bytes.Buffer
	err := clierrors.NewValidationError("bad input").
		WithSuggestion("try --help")

	OutputError(&buf, err, false)

	out := buf.String()
	if !strings.Contains(out, "Error: bad input") {
		t.Errorf("missing error message: %s", out)
	}
	if !strings.Contains(out, "Suggestion: try --help") {
		t.Errorf("missing suggestion: %s", out)
	}
}

func TestOutputError_Text_PlainError(t *testing.T) {
	var buf bytes.Buffer
	OutputError(&buf, fmt.Errorf("something broke"), false)

	out := buf.String()
	if !strings.Contains(out, "Error: something broke") {
		t.Errorf("missing error message: %s", out)
	}
}

func TestOutputError_Text_NoSuggestion(t *testing.T) {
	var buf bytes.Buffer
	err := clierrors.NewValidationError("bad input")
	OutputError(&buf, err, false)

	out := buf.String()
	if strings.Contains(out, "Suggestion:") {
		t.Errorf("should not show suggestion line: %s", out)
	}
}

func TestOutputError_JSON_CLIError(t *testing.T) {
	var buf bytes.Buffer
	err := clierrors.NewAuthError("invalid credentials").
		WithSuggestion("check token")

	OutputError(&buf, err, true)

	var got map[string]map[string]interface{}
	if e := json.Unmarshal(buf.Bytes(), &got); e != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", e, buf.String())
	}
	if got["error"]["code"] != "AUTH_ERROR" {
		t.Errorf("code = %v, want AUTH_ERROR", got["error"]["code"])
	}
	if got["error"]["message"] != "invalid credentials" {
		t.Errorf("message = %v", got["error"]["message"])
	}
	if got["error"]["suggestion"] != "check token" {
		t.Errorf("suggestion = %v", got["error"]["suggestion"])
	}
}

func TestOutputError_JSON_PlainError(t *testing.T) {
	var buf bytes.Buffer
	OutputError(&buf, errors.New("boom"), true)

	var got map[string]map[string]interface{}
	if e := json.Unmarshal(buf.Bytes(), &got); e != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", e, buf.String())
	}
	if got["error"]["code"] != "GENERAL_ERROR" {
		t.Errorf("code = %v, want GENERAL_ERROR", got["error"]["code"])
	}
}

// --- Formatter integration tests ---

func newTestFormatter(asJSON bool, jqExpr string) (*Formatter, *bytes.Buffer) {
	tio := iostreams.Test()
	f := NewFormatter(tio.IOStreams, asJSON, jqExpr)
	return f, tio.OutBuf
}

func TestFormatter_OutputData_JSON(t *testing.T) {
	f, buf := newTestFormatter(true, "")
	data := map[string]string{"key": "PROJ-1"}

	if err := f.OutputData(data, nil); err != nil {
		t.Fatal(err)
	}

	var got map[string]string
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got["key"] != "PROJ-1" {
		t.Errorf("key = %v", got["key"])
	}
}

func TestFormatter_OutputData_Table(t *testing.T) {
	f, buf := newTestFormatter(false, "")
	data := map[string]string{"key": "PROJ-1"}

	err := f.OutputData(data, func(t table.Writer) {
		t.AppendRow(table.Row{"PROJ-1", "Summary text"})
	})
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, "PROJ-1") {
		t.Errorf("table missing PROJ-1: %s", out)
	}
	if !strings.Contains(out, "Summary text") {
		t.Errorf("table missing Summary text: %s", out)
	}
}

func TestFormatter_OutputList_JSON(t *testing.T) {
	f, buf := newTestFormatter(true, "")
	items := []string{"a", "b"}
	pg := &PaginationMeta{Offset: 0, Limit: 10, HasNextPage: false}

	if err := f.OutputList(items, pg, nil); err != nil {
		t.Fatal(err)
	}

	var got struct {
		Data       []string        `json:"data"`
		Pagination *PaginationMeta `json:"pagination"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(got.Data) != 2 {
		t.Errorf("data length = %d", len(got.Data))
	}
	if got.Pagination.HasNextPage {
		t.Error("expected has_next_page=false")
	}
}

func TestFormatter_OutputList_Table(t *testing.T) {
	f, buf := newTestFormatter(false, "")

	err := f.OutputList(nil, nil, func(t table.Writer) {
		t.AppendRow(table.Row{"row1"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "row1") {
		t.Errorf("table missing row1: %s", buf.String())
	}
}

func TestFormatter_OutputMutation_JSON(t *testing.T) {
	f, buf := newTestFormatter(true, "")
	extras := map[string]interface{}{"key": "PROJ-1", "action": "created"}

	if err := f.OutputMutation(extras, nil); err != nil {
		t.Fatal(err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got["ok"] != true {
		t.Errorf("ok = %v", got["ok"])
	}
	if got["key"] != "PROJ-1" {
		t.Errorf("key = %v", got["key"])
	}
}

func TestFormatter_OutputMutation_Table(t *testing.T) {
	f, buf := newTestFormatter(false, "")

	err := f.OutputMutation(nil, func(t table.Writer) {
		t.AppendRow(table.Row{"Created PROJ-1"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Created PROJ-1") {
		t.Errorf("table output: %s", buf.String())
	}
}

func TestFormatter_OutputDryRun_JSON(t *testing.T) {
	f, buf := newTestFormatter(true, "")
	payload := map[string]string{"summary": "Test"}

	if err := f.OutputDryRun(payload, "passed", nil); err != nil {
		t.Fatal(err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if got["dry_run"] != true {
		t.Errorf("dry_run = %v", got["dry_run"])
	}
	if got["validation"] != "passed" {
		t.Errorf("validation = %v", got["validation"])
	}
}

func TestFormatter_OutputDryRun_Table(t *testing.T) {
	f, buf := newTestFormatter(false, "")

	err := f.OutputDryRun(nil, "", func(t table.Writer) {
		t.AppendRow(table.Row{"DRY RUN", "summary: Test"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "DRY RUN") {
		t.Errorf("table output: %s", buf.String())
	}
}

func TestFormatter_JQ_ImpliesJSON(t *testing.T) {
	f, _ := newTestFormatter(false, ".key")
	if !f.IsJSON() {
		t.Error("jqExpr should imply JSON mode")
	}
}

func TestFormatter_JQ_FilterOnOutputData(t *testing.T) {
	f, buf := newTestFormatter(false, ".key")
	data := map[string]string{"key": "PROJ-1", "summary": "Ignored"}

	if err := f.OutputData(data, nil); err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	if got != "PROJ-1" {
		t.Errorf("jq result = %q, want PROJ-1", got)
	}
}

func TestFormatter_JQ_FilterOnOutputList(t *testing.T) {
	f, buf := newTestFormatter(true, ".data[].k")
	items := []map[string]string{{"k": "A"}, {"k": "B"}}
	pg := &PaginationMeta{Offset: 0, Limit: 10}

	if err := f.OutputList(items, pg, nil); err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	if got != "A\nB" {
		t.Errorf("jq result = %q, want 'A\\nB'", got)
	}
}

func TestFormatter_JQ_FilterOnOutputMutation(t *testing.T) {
	f, buf := newTestFormatter(true, ".ok")
	extras := map[string]interface{}{"key": "PROJ-1"}

	if err := f.OutputMutation(extras, nil); err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	if got != "true" {
		t.Errorf("jq .ok = %q, want true", got)
	}
}

func TestFormatter_IsJSON(t *testing.T) {
	tests := []struct {
		name   string
		asJSON bool
		jq     string
		want   bool
	}{
		{"text mode", false, "", false},
		{"json mode", true, "", true},
		{"jq implies json", false, ".key", true},
		{"json + jq", true, ".key", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _ := newTestFormatter(tt.asJSON, tt.jq)
			if f.IsJSON() != tt.want {
				t.Errorf("IsJSON() = %v, want %v", f.IsJSON(), tt.want)
			}
		})
	}
}

// --- No ANSI in non-TTY ---

func TestNoANSI_WhenNotTTY(t *testing.T) {
	// iostreams.Test() creates non-TTY streams with color disabled.
	tio := iostreams.Test()
	f := NewFormatter(tio.IOStreams, true, "")

	data := map[string]string{"status": "In Progress"}
	if err := f.OutputData(data, nil); err != nil {
		t.Fatal(err)
	}

	out := tio.OutBuf.String()
	// ANSI escape starts with \x1b[
	if strings.Contains(out, "\x1b[") {
		t.Errorf("JSON output contains ANSI codes: %q", out)
	}
}
