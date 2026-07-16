package adf

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Convert parses a CommonMark Markdown string and returns an ADF Document.
// Plain text (no Markdown syntax) passes through as a single paragraph with a text node.
// Empty input returns an empty document.
func Convert(markdown string) (*Node, error) {
	if markdown == "" {
		return Document(), nil
	}

	source := []byte(markdown)

	md := goldmark.New(
		goldmark.WithExtensions(extension.Strikethrough, extension.Table),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	c := &converter{source: source}
	content := c.convertChildren(doc, source)

	return Document(content...), nil
}

// converter holds state during AST walking.
type converter struct {
	source []byte
}

// convertChildren processes all children of an AST node.
func (c *converter) convertChildren(n ast.Node, source []byte) []*Node {
	var nodes []*Node
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if result := c.convertNode(child, source); result != nil {
			nodes = append(nodes, result)
		}
	}
	return nodes
}

// convertNode dispatches a single AST node to its ADF equivalent.
func (c *converter) convertNode(n ast.Node, source []byte) *Node {
	switch n.Kind() {
	case ast.KindParagraph, ast.KindTextBlock:
		// TextBlock is used by goldmark for tight list items instead of Paragraph.
		// Both map to ADF paragraph.
		content := c.convertInlineChildren(n, source, nil)
		if len(content) == 0 {
			return Paragraph()
		}
		return Paragraph(content...)

	case ast.KindHeading:
		heading := n.(*ast.Heading)
		content := c.convertInlineChildren(n, source, nil)
		return Heading(heading.Level, content...)

	case ast.KindList:
		list := n.(*ast.List)
		items := c.convertChildren(n, source)
		if list.IsOrdered() {
			return OrderedList(items...)
		}
		return BulletList(items...)

	case ast.KindListItem:
		content := c.convertChildren(n, source)
		return ListItem(content...)

	case ast.KindFencedCodeBlock:
		fcb := n.(*ast.FencedCodeBlock)
		lang := ""
		if fcb.Language(source) != nil {
			lang = string(fcb.Language(source))
		}
		codeText := c.extractCodeBlockText(fcb, source)
		if codeText == "" {
			return CodeBlock(lang)
		}
		return CodeBlock(lang, Text(codeText))

	case ast.KindCodeBlock:
		// Indented code blocks (no language)
		cb := n.(*ast.CodeBlock)
		codeText := c.extractBaseCodeBlockText(cb, source)
		if codeText == "" {
			return CodeBlock("")
		}
		return CodeBlock("", Text(codeText))

	case ast.KindBlockquote:
		content := c.convertChildren(n, source)
		return Blockquote(content...)

	case ast.KindThematicBreak:
		return Rule()

	default:
		// GFM table extension nodes
		switch n.Kind() {
		case east.KindTable:
			return c.convertTable(n, source)
		case east.KindTableHeader:
			return c.convertTableRow(n, source, true)
		case east.KindTableRow:
			return c.convertTableRow(n, source, false)
		}

		// For unknown block-level nodes, try to extract children.
		if n.HasChildren() {
			content := c.convertChildren(n, source)
			if len(content) > 0 {
				return content[0]
			}
		}
		return nil
	}
}

// convertInlineChildren processes inline children of a block node, accumulating marks.
func (c *converter) convertInlineChildren(n ast.Node, source []byte, marks []Mark) []*Node {
	var nodes []*Node
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		nodes = append(nodes, c.convertInline(child, source, marks)...)
	}
	return nodes
}

// convertInline converts an inline AST node to ADF text nodes with marks.
func (c *converter) convertInline(n ast.Node, source []byte, marks []Mark) []*Node {
	switch n.Kind() {
	case ast.KindText:
		tn := n.(*ast.Text)
		t := string(tn.Segment.Value(source))
		if t == "" {
			return nil
		}
		node := Text(t, copyMarks(marks)...)
		var nodes []*Node
		nodes = append(nodes, node)
		if tn.SoftLineBreak() {
			// CommonMark soft line breaks are treated as whitespace (single space).
			// ADF represents line breaks with hardBreak nodes, not embedded newlines
			// in text nodes. A soft break maps to a space per CommonMark semantics.
			nodes = append(nodes, Text(" "))
		}
		if tn.HardLineBreak() {
			nodes = append(nodes, HardBreak())
		}
		return nodes

	case ast.KindString:
		t := string(n.(*ast.String).Value)
		if t == "" {
			return nil
		}
		return []*Node{Text(t, copyMarks(marks)...)}

	case ast.KindEmphasis:
		em := n.(*ast.Emphasis)
		var mark Mark
		if em.Level == 2 {
			mark = Strong()
		} else {
			mark = Em()
		}
		return c.convertInlineChildren(n, source, append(copyMarks(marks), mark))

	case ast.KindCodeSpan:
		// Inline code: extract raw text segments.
		t := c.extractInlineCodeText(n, source)
		if t == "" {
			return nil
		}
		// Per the ADF spec, the `code` mark may only be combined with `link`.
		// Inherited `em`/`strong`/`strike` marks (e.g. an inline code span inside
		// an italic footer) must be dropped, or Jira rejects the document with
		// INVALID_INPUT. Keep only link marks alongside code.
		return []*Node{Text(t, append(keepLinkMarks(marks), Code())...)}

	case ast.KindLink:
		link := n.(*ast.Link)
		href := string(link.Destination)
		childMarks := append(copyMarks(marks), Link(href))
		return c.convertInlineChildren(n, source, childMarks)

	case ast.KindAutoLink:
		al := n.(*ast.AutoLink)
		url := string(al.URL(source))
		linkMarks := append(copyMarks(marks), Link(url))
		return []*Node{Text(url, linkMarks...)}

	default:
		// Check for strikethrough extension
		if n.Kind() == east.KindStrikethrough {
			return c.convertInlineChildren(n, source, append(copyMarks(marks), Strike()))
		}

		// Unknown inline: try children
		if n.HasChildren() {
			return c.convertInlineChildren(n, source, marks)
		}
		return nil
	}
}

// extractCodeBlockText concatenates all lines of a fenced code block.
func (c *converter) extractCodeBlockText(n *ast.FencedCodeBlock, source []byte) string {
	var buf []byte
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf = append(buf, seg.Value(source)...)
	}
	return string(buf)
}

// extractBaseCodeBlockText concatenates all lines of an indented code block.
func (c *converter) extractBaseCodeBlockText(n *ast.CodeBlock, source []byte) string {
	var buf []byte
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf = append(buf, seg.Value(source)...)
	}
	return string(buf)
}

// extractInlineCodeText extracts text from an inline code span's raw segments.
func (c *converter) extractInlineCodeText(n ast.Node, source []byte) string {
	// Code spans don't have children in the usual sense; iterate over
	// the node's children which are ast.Text nodes with raw content.
	var buf []byte
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			buf = append(buf, t.Segment.Value(source)...)
		}
	}
	return string(buf)
}

// convertTable converts a GFM table AST node to an ADF table.
func (c *converter) convertTable(n ast.Node, source []byte) *Node {
	var rows []*Node
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		if row := c.convertNode(child, source); row != nil {
			rows = append(rows, row)
		}
	}
	return Table(rows...)
}

// convertTableRow converts a GFM table header or row to an ADF tableRow.
// Header cells become tableHeader nodes; body cells become tableCell nodes.
func (c *converter) convertTableRow(n ast.Node, source []byte, isHeader bool) *Node {
	var cells []*Node
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		content := c.convertInlineChildren(child, source, nil)
		para := Paragraph(content...)
		if isHeader {
			cells = append(cells, TableHeader(para))
		} else {
			cells = append(cells, TableCell(para))
		}
	}
	return TableRow(cells...)
}

// copyMarks returns a copy of the marks slice to prevent mutation.
func copyMarks(marks []Mark) []Mark {
	if len(marks) == 0 {
		return nil
	}
	cp := make([]Mark, len(marks))
	copy(cp, marks)
	return cp
}

// keepLinkMarks returns a copy of marks containing only link marks — the only
// mark the ADF spec permits alongside the code mark. Used to sanitize an inline
// code span that inherits em/strong/strike from an enclosing span.
func keepLinkMarks(marks []Mark) []Mark {
	var out []Mark
	for _, m := range marks {
		if m.Type == MarkLink {
			out = append(out, m)
		}
	}
	return out
}
