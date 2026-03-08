package adf

import (
	"encoding/json"
	"testing"
)

func TestToPlaintext(t *testing.T) {
	tests := []struct {
		name string
		doc  json.RawMessage
		want string
	}{
		{
			name: "nil input",
			doc:  nil,
			want: "",
		},
		{
			name: "empty input",
			doc:  json.RawMessage{},
			want: "",
		},
		{
			name: "invalid JSON returns raw string",
			doc:  json.RawMessage(`not valid json`),
			want: "not valid json",
		},
		{
			name: "simple paragraph",
			doc:  mustMarshal(Document(Paragraph(Text("Hello world")))),
			want: "Hello world",
		},
		{
			name: "multiple paragraphs",
			doc: mustMarshal(Document(
				Paragraph(Text("First paragraph")),
				Paragraph(Text("Second paragraph")),
			)),
			want: "First paragraph\n\nSecond paragraph",
		},
		{
			name: "heading",
			doc: mustMarshal(Document(
				Heading(1, Text("Title")),
				Paragraph(Text("Body text")),
			)),
			want: "Title\n\nBody text",
		},
		{
			name: "bullet list",
			doc: mustMarshal(Document(
				BulletList(
					ListItem(Paragraph(Text("Item one"))),
					ListItem(Paragraph(Text("Item two"))),
					ListItem(Paragraph(Text("Item three"))),
				),
			)),
			want: "- Item one\n- Item two\n- Item three",
		},
		{
			name: "ordered list",
			doc: mustMarshal(Document(
				OrderedList(
					ListItem(Paragraph(Text("First"))),
					ListItem(Paragraph(Text("Second"))),
					ListItem(Paragraph(Text("Third"))),
				),
			)),
			want: "1. First\n2. Second\n3. Third",
		},
		{
			name: "nested bullet list",
			doc: mustMarshal(Document(
				BulletList(
					ListItem(
						Paragraph(Text("Parent")),
						BulletList(
							ListItem(Paragraph(Text("Child one"))),
							ListItem(Paragraph(Text("Child two"))),
						),
					),
				),
			)),
			want: "- Parent\n  - Child one\n  - Child two",
		},
		{
			name: "nested ordered in bullet",
			doc: mustMarshal(Document(
				BulletList(
					ListItem(
						Paragraph(Text("Parent")),
						OrderedList(
							ListItem(Paragraph(Text("Step one"))),
							ListItem(Paragraph(Text("Step two"))),
						),
					),
				),
			)),
			want: "- Parent\n  1. Step one\n  2. Step two",
		},
		{
			name: "code block",
			doc: mustMarshal(Document(
				CodeBlock("go", Text("func main() {\n\tfmt.Println(\"hello\")\n}")),
			)),
			want: "    func main() {\n    \tfmt.Println(\"hello\")\n    }",
		},
		{
			name: "code block single line",
			doc:  mustMarshal(Document(CodeBlock("", Text("echo hello")))),
			want: "    echo hello",
		},
		{
			name: "blockquote",
			doc: mustMarshal(Document(
				Blockquote(Paragraph(Text("Quoted text"))),
			)),
			want: "> Quoted text",
		},
		{
			name: "blockquote multiple paragraphs",
			doc: mustMarshal(Document(
				Blockquote(
					Paragraph(Text("First quoted")),
					Paragraph(Text("Second quoted")),
				),
			)),
			want: "> First quoted\n\n> Second quoted",
		},
		{
			name: "horizontal rule",
			doc: mustMarshal(Document(
				Paragraph(Text("Above")),
				Rule(),
				Paragraph(Text("Below")),
			)),
			want: "Above\n\n---\n\nBelow",
		},
		{
			name: "table renders with separators",
			doc: mustMarshal(Document(
				Table(
					TableRow(
						TableHeader(Paragraph(Text("Name"))),
						TableHeader(Paragraph(Text("Value"))),
					),
					TableRow(
						TableCell(Paragraph(Text("foo"))),
						TableCell(Paragraph(Text("bar"))),
					),
				),
			)),
			want: "Name | Value\n--- | ---\nfoo | bar",
		},
		{
			name: "hard break",
			doc: mustMarshal(Document(
				Paragraph(Text("Line one"), HardBreak(), Text("Line two")),
			)),
			want: "Line one\nLine two",
		},
		{
			name: "bold text stripped",
			doc:  mustMarshal(Document(Paragraph(Text("Hello ", Strong()), Text("world")))),
			want: "Hello world",
		},
		{
			name: "italic text stripped",
			doc:  mustMarshal(Document(Paragraph(Text("Hello ", Em()), Text("world")))),
			want: "Hello world",
		},
		{
			name: "strike text stripped",
			doc:  mustMarshal(Document(Paragraph(Text("deleted", Strike())))),
			want: "deleted",
		},
		{
			name: "inline code wrapped in backticks",
			doc:  mustMarshal(Document(Paragraph(Text("Use "), Text("fmt.Println", Code()), Text(" here")))),
			want: "Use `fmt.Println` here",
		},
		{
			name: "link renders text and URL",
			doc:  mustMarshal(Document(Paragraph(Text("Visit "), Text("Google", Link("https://google.com"))))),
			want: "Visit Google (https://google.com)",
		},
		{
			name: "mixed marks on same text",
			doc:  mustMarshal(Document(Paragraph(Text("important", Strong(), Em())))),
			want: "important",
		},
		{
			name: "complex document",
			doc: mustMarshal(Document(
				Heading(1, Text("Release Notes")),
				Paragraph(Text("Version 2.0 is here.")),
				Heading(2, Text("Changes")),
				BulletList(
					ListItem(Paragraph(Text("New feature A"))),
					ListItem(Paragraph(Text("Bug fix B"))),
				),
				Paragraph(Text("Example code:")),
				CodeBlock("bash", Text("jira issue list")),
				Blockquote(Paragraph(Text("This is a quote"))),
				Rule(),
				Paragraph(Text("End.")),
			)),
			want: "Release Notes\n\nVersion 2.0 is here.\n\nChanges\n\n- New feature A\n- Bug fix B\n\nExample code:\n\n    jira issue list\n\n> This is a quote\n\n---\n\nEnd.",
		},
		{
			name: "empty document",
			doc:  mustMarshal(Document()),
			want: "",
		},
		{
			name: "paragraph with no children",
			doc:  mustMarshal(Document(Paragraph())),
			want: "",
		},
		{
			name: "deeply nested lists",
			doc: mustMarshal(Document(
				BulletList(
					ListItem(
						Paragraph(Text("Level 1")),
						BulletList(
							ListItem(
								Paragraph(Text("Level 2")),
								BulletList(
									ListItem(Paragraph(Text("Level 3"))),
								),
							),
						),
					),
				),
			)),
			want: "- Level 1\n  - Level 2\n    - Level 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToPlaintext(tt.doc)
			if got != tt.want {
				t.Errorf("ToPlaintext():\ngot:  %q\nwant: %q", got, tt.want)
			}
		})
	}
}

// mustMarshal is a test helper that marshals a Node to json.RawMessage.
func mustMarshal(n *Node) json.RawMessage {
	b, err := json.Marshal(n)
	if err != nil {
		panic("mustMarshal: " + err.Error())
	}
	return b
}
