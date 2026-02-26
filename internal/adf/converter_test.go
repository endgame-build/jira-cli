package adf

import (
	"encoding/json"
	"testing"
)

func TestConvert(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		check    func(t *testing.T, doc *Node)
	}{
		{
			name:     "empty input",
			markdown: "",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				if len(doc.Content) != 0 {
					t.Errorf("expected empty content, got %d nodes", len(doc.Content))
				}
			},
		},
		{
			name:     "plain text",
			markdown: "Hello world",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				p := doc.Content[0]
				assertNodeType(t, p, TypeParagraph)
				assertContentLen(t, p, 1)
				assertText(t, p.Content[0], "Hello world")
			},
		},
		{
			name:     "paragraph",
			markdown: "First paragraph.\n\nSecond paragraph.",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 2)
				assertNodeType(t, doc.Content[0], TypeParagraph)
				assertNodeType(t, doc.Content[1], TypeParagraph)
				assertTextInParagraph(t, doc.Content[0], "First paragraph.")
				assertTextInParagraph(t, doc.Content[1], "Second paragraph.")
			},
		},
		{
			name:     "heading levels",
			markdown: "# H1\n\n## H2\n\n### H3\n\n#### H4\n\n##### H5\n\n###### H6",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 6)
				for i, level := range []int{1, 2, 3, 4, 5, 6} {
					h := doc.Content[i]
					assertNodeType(t, h, TypeHeading)
					gotLevel, ok := h.Attrs["level"].(int)
					if !ok || gotLevel != level {
						t.Errorf("heading[%d]: expected level %d, got %v", i, level, h.Attrs["level"])
					}
				}
			},
		},
		{
			name:     "bold text",
			markdown: "This is **bold** text",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				p := doc.Content[0]
				// Should have: "This is ", "bold" (strong), " text"
				assertHasTextWithMark(t, p, "bold", MarkStrong)
			},
		},
		{
			name:     "italic text",
			markdown: "This is *italic* text",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				p := doc.Content[0]
				assertHasTextWithMark(t, p, "italic", MarkEm)
			},
		},
		{
			name:     "inline code",
			markdown: "Use `fmt.Println` here",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				p := doc.Content[0]
				assertHasTextWithMark(t, p, "fmt.Println", MarkCode)
			},
		},
		{
			name:     "link",
			markdown: "Click [here](https://example.com) now",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				p := doc.Content[0]
				found := false
				for _, n := range p.Content {
					if n.Type == TypeText && n.Text == "here" {
						for _, m := range n.Marks {
							if m.Type == MarkLink {
								if m.Attrs["href"] != "https://example.com" {
									t.Errorf("expected href https://example.com, got %v", m.Attrs["href"])
								}
								found = true
							}
						}
					}
				}
				if !found {
					t.Error("link mark not found on 'here' text")
				}
			},
		},
		{
			name:     "strikethrough",
			markdown: "This is ~~deleted~~ text",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				p := doc.Content[0]
				assertHasTextWithMark(t, p, "deleted", MarkStrike)
			},
		},
		{
			name:     "bullet list",
			markdown: "- Item 1\n- Item 2\n- Item 3",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				list := doc.Content[0]
				assertNodeType(t, list, TypeBulletList)
				assertContentLen(t, list, 3)
				for i, item := range list.Content {
					assertNodeType(t, item, TypeListItem)
					if len(item.Content) == 0 {
						t.Errorf("list item %d has no content", i)
					}
				}
			},
		},
		{
			name:     "ordered list",
			markdown: "1. First\n2. Second\n3. Third",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				list := doc.Content[0]
				assertNodeType(t, list, TypeOrderedList)
				assertContentLen(t, list, 3)
			},
		},
		{
			name:     "fenced code block with language",
			markdown: "```go\nfmt.Println(\"hello\")\n```",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				cb := doc.Content[0]
				assertNodeType(t, cb, TypeCodeBlock)
				if cb.Attrs["language"] != "go" {
					t.Errorf("expected language 'go', got %v", cb.Attrs["language"])
				}
				assertContentLen(t, cb, 1)
				assertText(t, cb.Content[0], "fmt.Println(\"hello\")\n")
			},
		},
		{
			name:     "fenced code block without language",
			markdown: "```\nsome code\n```",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				cb := doc.Content[0]
				assertNodeType(t, cb, TypeCodeBlock)
				if cb.Attrs != nil {
					t.Errorf("expected no attrs for code block without language, got %v", cb.Attrs)
				}
			},
		},
		{
			name:     "blockquote",
			markdown: "> This is a quote",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				bq := doc.Content[0]
				assertNodeType(t, bq, TypeBlockquote)
				if len(bq.Content) == 0 {
					t.Fatal("blockquote has no content")
				}
				assertNodeType(t, bq.Content[0], TypeParagraph)
			},
		},
		{
			name:     "horizontal rule",
			markdown: "Before\n\n---\n\nAfter",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 3)
				assertNodeType(t, doc.Content[0], TypeParagraph)
				assertNodeType(t, doc.Content[1], TypeRule)
				assertNodeType(t, doc.Content[2], TypeParagraph)
			},
		},
		{
			name:     "nested bold inside italic",
			markdown: "*italic and **bold italic** end*",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				p := doc.Content[0]
				// Find text with both em and strong marks
				found := false
				for _, n := range p.Content {
					if n.Type == TypeText && n.Text == "bold italic" {
						hasEm := false
						hasStrong := false
						for _, m := range n.Marks {
							if m.Type == MarkEm {
								hasEm = true
							}
							if m.Type == MarkStrong {
								hasStrong = true
							}
						}
						if hasEm && hasStrong {
							found = true
						}
					}
				}
				if !found {
					t.Error("expected text 'bold italic' with both em and strong marks")
				}
			},
		},
		{
			name:     "link inside list",
			markdown: "- Click [here](https://example.com)\n- Other item",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				list := doc.Content[0]
				assertNodeType(t, list, TypeBulletList)
				assertContentLen(t, list, 2)
				// First item should have a paragraph with link
				item := list.Content[0]
				assertNodeType(t, item, TypeListItem)
				if len(item.Content) == 0 {
					t.Fatal("list item has no content")
				}
				p := item.Content[0]
				assertNodeType(t, p, TypeParagraph)
				found := false
				for _, n := range p.Content {
					if n.Type == TypeText && n.Text == "here" {
						for _, m := range n.Marks {
							if m.Type == MarkLink {
								found = true
							}
						}
					}
				}
				if !found {
					t.Error("link not found in first list item")
				}
			},
		},
		{
			name:     "code inside bold",
			markdown: "**Use `code` here**",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				p := doc.Content[0]
				// Find text "code" with both strong and code marks
				found := false
				for _, n := range p.Content {
					if n.Type == TypeText && n.Text == "code" {
						hasStrong := false
						hasCode := false
						for _, m := range n.Marks {
							if m.Type == MarkStrong {
								hasStrong = true
							}
							if m.Type == MarkCode {
								hasCode = true
							}
						}
						if hasStrong && hasCode {
							found = true
						}
					}
				}
				if !found {
					t.Error("expected text 'code' with both strong and code marks")
				}
			},
		},
		{
			name:     "multi-line blockquote",
			markdown: "> Line one\n> Line two",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				bq := doc.Content[0]
				assertNodeType(t, bq, TypeBlockquote)
			},
		},
		{
			name:     "nested list",
			markdown: "- Item 1\n  - Sub-item A\n  - Sub-item B\n- Item 2",
			check: func(t *testing.T, doc *Node) {
				assertDocNode(t, doc)
				assertContentLen(t, doc, 1)
				list := doc.Content[0]
				assertNodeType(t, list, TypeBulletList)
				// First item should contain a paragraph and a nested list
				item1 := list.Content[0]
				assertNodeType(t, item1, TypeListItem)
				hasNestedList := false
				for _, child := range item1.Content {
					if child.Type == TypeBulletList {
						hasNestedList = true
						assertContentLen(t, child, 2)
					}
				}
				if !hasNestedList {
					t.Error("expected nested bullet list inside first item")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := Convert(tt.markdown)
			if err != nil {
				t.Fatalf("Convert() error: %v", err)
			}
			tt.check(t, doc)
		})
	}
}

func TestConvertJSONRoundTrip(t *testing.T) {
	// Verify that the output is valid JSON matching ADF schema shape
	markdown := "# Title\n\nHello **world** and *italic*.\n\n- item\n\n```go\nfmt.Println()\n```\n\n---"

	doc, err := Convert(markdown)
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}

	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	// Verify it's valid JSON
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	// Check top-level shape
	if raw["type"] != "doc" {
		t.Errorf("expected type 'doc', got %v", raw["type"])
	}
	if raw["version"] != float64(1) {
		t.Errorf("expected version 1, got %v", raw["version"])
	}
	content, ok := raw["content"].([]interface{})
	if !ok {
		t.Fatal("expected content array")
	}
	// Should have: heading, paragraph, bulletList, codeBlock, rule
	if len(content) != 5 {
		t.Errorf("expected 5 top-level nodes, got %d", len(content))
	}
}

func TestConvertEmptyDoc(t *testing.T) {
	doc, err := Convert("")
	if err != nil {
		t.Fatalf("Convert() error: %v", err)
	}

	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	expected := `{"type":"doc","version":1}`
	if string(b) != expected {
		t.Errorf("expected %s, got %s", expected, string(b))
	}
}

func TestNodeConstructors(t *testing.T) {
	tests := []struct {
		name  string
		node  *Node
		check func(t *testing.T, n *Node)
	}{
		{
			name: "Document",
			node: Document(Paragraph(Text("hello"))),
			check: func(t *testing.T, n *Node) {
				assertNodeType(t, n, TypeDoc)
				if n.Version != 1 {
					t.Errorf("expected version 1, got %d", n.Version)
				}
				assertContentLen(t, n, 1)
			},
		},
		{
			name: "Heading",
			node: Heading(3, Text("title")),
			check: func(t *testing.T, n *Node) {
				assertNodeType(t, n, TypeHeading)
				if n.Attrs["level"] != 3 {
					t.Errorf("expected level 3, got %v", n.Attrs["level"])
				}
			},
		},
		{
			name: "CodeBlock with language",
			node: CodeBlock("python", Text("print(1)")),
			check: func(t *testing.T, n *Node) {
				assertNodeType(t, n, TypeCodeBlock)
				if n.Attrs["language"] != "python" {
					t.Errorf("expected language 'python', got %v", n.Attrs["language"])
				}
			},
		},
		{
			name: "CodeBlock without language",
			node: CodeBlock(""),
			check: func(t *testing.T, n *Node) {
				assertNodeType(t, n, TypeCodeBlock)
				if n.Attrs != nil {
					t.Errorf("expected nil attrs, got %v", n.Attrs)
				}
			},
		},
		{
			name: "Text with marks",
			node: Text("bold", Strong()),
			check: func(t *testing.T, n *Node) {
				assertNodeType(t, n, TypeText)
				if n.Text != "bold" {
					t.Errorf("expected text 'bold', got %q", n.Text)
				}
				if len(n.Marks) != 1 || n.Marks[0].Type != MarkStrong {
					t.Errorf("expected strong mark, got %v", n.Marks)
				}
			},
		},
		{
			name: "Text without marks",
			node: Text("plain"),
			check: func(t *testing.T, n *Node) {
				if n.Marks != nil {
					t.Errorf("expected nil marks, got %v", n.Marks)
				}
			},
		},
		{
			name: "Link mark",
			node: Text("click", Link("https://example.com")),
			check: func(t *testing.T, n *Node) {
				if len(n.Marks) != 1 {
					t.Fatalf("expected 1 mark, got %d", len(n.Marks))
				}
				m := n.Marks[0]
				if m.Type != MarkLink {
					t.Errorf("expected link mark, got %v", m.Type)
				}
				if m.Attrs["href"] != "https://example.com" {
					t.Errorf("expected href, got %v", m.Attrs["href"])
				}
			},
		},
		{
			name: "HardBreak",
			node: HardBreak(),
			check: func(t *testing.T, n *Node) {
				assertNodeType(t, n, TypeHardBreak)
			},
		},
		{
			name: "Rule",
			node: Rule(),
			check: func(t *testing.T, n *Node) {
				assertNodeType(t, n, TypeRule)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, tt.node)
		})
	}
}

func TestMarkConstructors(t *testing.T) {
	tests := []struct {
		name string
		mark Mark
		typ  MarkType
	}{
		{"Strong", Strong(), MarkStrong},
		{"Em", Em(), MarkEm},
		{"Code", Code(), MarkCode},
		{"Strike", Strike(), MarkStrike},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mark.Type != tt.typ {
				t.Errorf("expected mark type %v, got %v", tt.typ, tt.mark.Type)
			}
			if tt.mark.Attrs != nil {
				t.Errorf("expected nil attrs, got %v", tt.mark.Attrs)
			}
		})
	}
}

// --- test helpers ---

func assertDocNode(t *testing.T, n *Node) {
	t.Helper()
	if n == nil {
		t.Fatal("expected non-nil node")
	}
	if n.Type != TypeDoc {
		t.Errorf("expected type 'doc', got %q", n.Type)
	}
	if n.Version != 1 {
		t.Errorf("expected version 1, got %d", n.Version)
	}
}

func assertNodeType(t *testing.T, n *Node, expected NodeType) {
	t.Helper()
	if n == nil {
		t.Fatalf("expected non-nil node of type %q", expected)
	}
	if n.Type != expected {
		t.Errorf("expected type %q, got %q", expected, n.Type)
	}
}

func assertContentLen(t *testing.T, n *Node, expected int) {
	t.Helper()
	if len(n.Content) != expected {
		t.Errorf("expected %d content nodes, got %d", expected, len(n.Content))
	}
}

func assertText(t *testing.T, n *Node, expected string) {
	t.Helper()
	assertNodeType(t, n, TypeText)
	if n.Text != expected {
		t.Errorf("expected text %q, got %q", expected, n.Text)
	}
}

func assertTextInParagraph(t *testing.T, p *Node, expected string) {
	t.Helper()
	assertNodeType(t, p, TypeParagraph)
	// Gather all text from paragraph children
	var text string
	for _, child := range p.Content {
		if child.Type == TypeText {
			text += child.Text
		}
	}
	if text != expected {
		t.Errorf("expected paragraph text %q, got %q", expected, text)
	}
}

func assertHasTextWithMark(t *testing.T, p *Node, text string, markType MarkType) {
	t.Helper()
	for _, n := range p.Content {
		if n.Type == TypeText && n.Text == text {
			for _, m := range n.Marks {
				if m.Type == markType {
					return
				}
			}
		}
	}
	t.Errorf("expected text %q with mark %q not found in paragraph", text, markType)
}
