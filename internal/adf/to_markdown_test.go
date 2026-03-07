package adf

import (
	"encoding/json"
	"testing"
)

func TestToMarkdown(t *testing.T) {
	tests := []struct {
		name string
		doc  json.RawMessage
		want string
	}{
		// --- Edge cases ---
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
			name: "empty document",
			doc:  mustMarshal(Document()),
			want: "",
		},
		{
			name: "paragraph with no children",
			doc:  mustMarshal(Document(Paragraph())),
			want: "",
		},

		// --- Individual node types ---
		{
			name: "heading level 1",
			doc:  mustMarshal(Document(Heading(1, Text("Title")))),
			want: "# Title",
		},
		{
			name: "heading level 2",
			doc:  mustMarshal(Document(Heading(2, Text("Subtitle")))),
			want: "## Subtitle",
		},
		{
			name: "heading level 3",
			doc:  mustMarshal(Document(Heading(3, Text("Section")))),
			want: "### Section",
		},
		{
			name: "heading level 6",
			doc:  mustMarshal(Document(Heading(6, Text("Deep")))),
			want: "###### Deep",
		},
		{
			name: "simple paragraph",
			doc:  mustMarshal(Document(Paragraph(Text("Hello world")))),
			want: "Hello world",
		},
		{
			name: "multiple paragraphs",
			doc: mustMarshal(Document(
				Paragraph(Text("First")),
				Paragraph(Text("Second")),
			)),
			want: "First\n\nSecond",
		},
		{
			name: "bullet list",
			doc: mustMarshal(Document(
				BulletList(
					ListItem(Paragraph(Text("Alpha"))),
					ListItem(Paragraph(Text("Beta"))),
					ListItem(Paragraph(Text("Gamma"))),
				),
			)),
			want: "- Alpha\n- Beta\n- Gamma",
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
							ListItem(Paragraph(Text("Child A"))),
							ListItem(Paragraph(Text("Child B"))),
						),
					),
				),
			)),
			want: "- Parent\n  - Child A\n  - Child B",
		},
		{
			name: "deeply nested bullet list",
			doc: mustMarshal(Document(
				BulletList(
					ListItem(
						Paragraph(Text("L1")),
						BulletList(
							ListItem(
								Paragraph(Text("L2")),
								BulletList(
									ListItem(Paragraph(Text("L3"))),
								),
							),
						),
					),
				),
			)),
			want: "- L1\n  - L2\n    - L3",
		},
		{
			name: "nested ordered in bullet",
			doc: mustMarshal(Document(
				BulletList(
					ListItem(
						Paragraph(Text("Parent")),
						OrderedList(
							ListItem(Paragraph(Text("Step 1"))),
							ListItem(Paragraph(Text("Step 2"))),
						),
					),
				),
			)),
			want: "- Parent\n   1. Step 1\n   2. Step 2",
		},
		{
			name: "code block with language",
			doc: mustMarshal(Document(
				CodeBlock("go", Text("func main() {}")),
			)),
			want: "```go\nfunc main() {}\n```",
		},
		{
			name: "code block without language",
			doc:  mustMarshal(Document(CodeBlock("", Text("echo hello")))),
			want: "```\necho hello\n```",
		},
		{
			name: "code block multiline",
			doc: mustMarshal(Document(
				CodeBlock("python", Text("def foo():\n    return 42\n")),
			)),
			want: "```python\ndef foo():\n    return 42\n```",
		},
		{
			name: "blockquote single paragraph",
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
			name: "hard break",
			doc: mustMarshal(Document(
				Paragraph(Text("Line one"), HardBreak(), Text("Line two")),
			)),
			want: "Line one\nLine two",
		},

		// --- Mark types ---
		{
			name: "bold text",
			doc:  mustMarshal(Document(Paragraph(Text("Hello "), Text("bold", Strong()), Text(" world")))),
			want: "Hello **bold** world",
		},
		{
			name: "italic text",
			doc:  mustMarshal(Document(Paragraph(Text("Hello "), Text("italic", Em()), Text(" world")))),
			want: "Hello *italic* world",
		},
		{
			name: "inline code",
			doc:  mustMarshal(Document(Paragraph(Text("Use "), Text("fmt.Println", Code()), Text(" here")))),
			want: "Use `fmt.Println` here",
		},
		{
			name: "link",
			doc:  mustMarshal(Document(Paragraph(Text("Visit "), Text("Google", Link("https://google.com"))))),
			want: "Visit [Google](https://google.com)",
		},
		{
			name: "strikethrough",
			doc:  mustMarshal(Document(Paragraph(Text("This is "), Text("deleted", Strike()), Text(" text")))),
			want: "This is ~~deleted~~ text",
		},
		{
			name: "combined marks bold and italic",
			doc:  mustMarshal(Document(Paragraph(Text("important", Strong(), Em())))),
			want: "***important***",
		},
		{
			name: "combined marks bold and code",
			doc:  mustMarshal(Document(Paragraph(Text("cmd", Strong(), Code())))),
			want: "`**cmd**`",
		},

		// --- Unknown node type fallback ---
		{
			name: "unknown node type extracts text",
			doc: func() json.RawMessage {
				n := &Node{
					Type: "table",
					Content: []*Node{
						{
							Type: "tableRow",
							Content: []*Node{
								{
									Type: "tableCell",
									Content: []*Node{
										Paragraph(Text("cell content")),
									},
								},
							},
						},
					},
				}
				doc := Document(n)
				b, _ := json.Marshal(doc)
				return b
			}(),
			want: "cell content",
		},
		{
			name: "unknown node with no children",
			doc: func() json.RawMessage {
				n := &Node{Type: "media"}
				doc := Document(n)
				b, _ := json.Marshal(doc)
				return b
			}(),
			want: "",
		},

		// --- Complex document ---
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
			want: "# Release Notes\n\n" +
				"Version 2.0 is here.\n\n" +
				"## Changes\n\n" +
				"- New feature A\n- Bug fix B\n\n" +
				"Example code:\n\n" +
				"```bash\njira issue list\n```\n\n" +
				"> This is a quote\n\n" +
				"---\n\n" +
				"End.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToMarkdown(tt.doc)
			if got != tt.want {
				t.Errorf("ToMarkdown():\ngot:  %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestToMarkdownRoundTrip(t *testing.T) {
	// ADF → Markdown → ADF should produce semantically equivalent structure
	// for all supported node types.
	tests := []struct {
		name string
		doc  *Node
	}{
		{
			name: "heading and paragraph",
			doc: Document(
				Heading(2, Text("Title")),
				Paragraph(Text("Body text.")),
			),
		},
		{
			name: "bold and italic",
			doc: Document(
				Paragraph(Text("Hello "), Text("bold", Strong()), Text(" and "), Text("italic", Em()), Text(".")),
			),
		},
		{
			name: "inline code",
			doc: Document(
				Paragraph(Text("Use "), Text("go test", Code()), Text(" to run.")),
			),
		},
		{
			name: "link",
			doc: Document(
				Paragraph(Text("Visit "), Text("example", Link("https://example.com")), Text(".")),
			),
		},
		{
			name: "strikethrough",
			doc: Document(
				Paragraph(Text("This "), Text("removed", Strike()), Text(" text.")),
			),
		},
		{
			name: "bullet list",
			doc: Document(
				BulletList(
					ListItem(Paragraph(Text("Item A"))),
					ListItem(Paragraph(Text("Item B"))),
				),
			),
		},
		{
			name: "ordered list",
			doc: Document(
				OrderedList(
					ListItem(Paragraph(Text("First"))),
					ListItem(Paragraph(Text("Second"))),
				),
			),
		},
		{
			name: "code block",
			doc: Document(
				CodeBlock("go", Text("fmt.Println(\"hello\")\n")),
			),
		},
		{
			name: "blockquote",
			doc: Document(
				Blockquote(Paragraph(Text("A wise quote"))),
			),
		},
		{
			name: "horizontal rule",
			doc: Document(
				Paragraph(Text("Before")),
				Rule(),
				Paragraph(Text("After")),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Step 1: ADF → JSON
			rawADF := mustMarshal(tt.doc)

			// Step 2: ADF JSON → Markdown
			md := ToMarkdown(rawADF)
			if md == "" {
				t.Fatal("ToMarkdown returned empty string")
			}

			// Step 3: Markdown → ADF (via Convert)
			roundTripped, err := Convert(md)
			if err != nil {
				t.Fatalf("Convert(ToMarkdown()) error: %v", err)
			}

			// Step 4: New ADF → JSON for comparison
			roundTrippedJSON := mustMarshal(roundTripped)

			// Step 5: Compare structure by unmarshaling both
			var original, result map[string]interface{}
			if err := json.Unmarshal(rawADF, &original); err != nil {
				t.Fatalf("unmarshal original: %v", err)
			}
			if err := json.Unmarshal(roundTrippedJSON, &result); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}

			// Compare top-level content node count
			origContent, _ := original["content"].([]interface{})
			resultContent, _ := result["content"].([]interface{})
			if len(origContent) != len(resultContent) {
				t.Errorf("content node count: original=%d, round-tripped=%d\noriginal MD: %q", len(origContent), len(resultContent), md)
			}

			// Compare node types at the top level
			for i := 0; i < len(origContent) && i < len(resultContent); i++ {
				origNode, _ := origContent[i].(map[string]interface{})
				resultNode, _ := resultContent[i].(map[string]interface{})
				if origNode["type"] != resultNode["type"] {
					t.Errorf("node[%d] type: original=%v, round-tripped=%v", i, origNode["type"], resultNode["type"])
				}
			}
		})
	}
}
