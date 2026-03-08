package adf

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ToMarkdown converts an ADF document (as raw JSON) into a Markdown string.
// This is the inverse of Convert() (Markdown→ADF). Unlike ToPlaintext, it
// produces standard CommonMark Markdown with proper syntax for headings,
// lists, code blocks, inline marks, etc.
//
// Returns empty string for nil/empty input. Returns an error for invalid JSON
// rather than silently falling back, since this feeds a file-writing pipeline
// where data integrity matters.
func ToMarkdown(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}

	var root Node
	if err := json.Unmarshal(raw, &root); err != nil {
		return "", fmt.Errorf("invalid ADF document: %w", err)
	}

	var sb strings.Builder
	mdRenderNode(&sb, &root, 0)
	return strings.TrimRight(sb.String(), "\n"), nil
}

// mdRenderNode recursively walks the ADF tree, producing Markdown output.
// depth tracks nesting level for list indentation.
func mdRenderNode(sb *strings.Builder, n *Node, depth int) {
	if n == nil {
		return
	}

	switch n.Type {
	case TypeDoc:
		for i, child := range n.Content {
			if i > 0 {
				sb.WriteString("\n")
			}
			mdRenderNode(sb, child, depth)
		}

	case TypeHeading:
		level := 1
		if l, ok := n.Attrs["level"]; ok {
			switch v := l.(type) {
			case float64:
				level = int(v)
			case int:
				level = v
			}
		}
		sb.WriteString(strings.Repeat("#", level))
		sb.WriteString(" ")
		mdRenderInline(sb, n)
		sb.WriteString("\n")

	case TypeParagraph:
		mdRenderInline(sb, n)
		sb.WriteString("\n")

	case TypeBulletList:
		for _, child := range n.Content {
			mdRenderNode(sb, child, depth)
		}

	case TypeOrderedList:
		for i, child := range n.Content {
			mdRenderOrderedItem(sb, child, depth, i+1)
		}

	case TypeListItem:
		// Default bullet list item rendering (2 chars: "- ")
		indent := strings.Repeat("  ", depth)
		for i, child := range n.Content {
			if i == 0 {
				sb.WriteString(indent)
				sb.WriteString("- ")
				mdRenderListChildContent(sb, child, depth)
			} else {
				// Nested list or continuation
				mdRenderNode(sb, child, depth+1)
			}
		}

	case TypeCodeBlock:
		lang := ""
		if l, ok := n.Attrs["language"]; ok {
			if s, ok := l.(string); ok {
				lang = s
			}
		}
		sb.WriteString("```")
		sb.WriteString(lang)
		sb.WriteString("\n")
		endsWithNewline := true // fence line ends with \n
		for _, child := range n.Content {
			if child.Type == TypeText {
				sb.WriteString(child.Text)
				endsWithNewline = len(child.Text) > 0 && child.Text[len(child.Text)-1] == '\n'
			}
		}
		// Ensure the closing fence is on its own line
		if !endsWithNewline {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n")

	case TypeBlockquote:
		for i, child := range n.Content {
			if i > 0 {
				sb.WriteString("\n")
			}
			mdRenderBlockquoteChild(sb, child)
		}

	case TypeTable:
		mdRenderTable(sb, n)

	case TypeRule:
		sb.WriteString("---\n")

	case TypeText:
		sb.WriteString(mdRenderTextWithMarks(n))

	case TypeHardBreak:
		sb.WriteString("\n")

	default:
		// Unknown node types: concatenate text content of all child text nodes
		mdRenderFallbackText(sb, n)
	}
}

// mdRenderOrderedItem renders a list item with ordered list prefix.
func mdRenderOrderedItem(sb *strings.Builder, n *Node, depth int, num int) {
	if n == nil {
		return
	}
	indent := strings.Repeat("   ", depth)
	for i, child := range n.Content {
		if i == 0 {
			sb.WriteString(indent)
			sb.WriteString(fmt.Sprintf("%d. ", num))
			mdRenderListChildContent(sb, child, depth)
		} else {
			// Nested list or continuation
			mdRenderNode(sb, child, depth+1)
		}
	}
}

// mdRenderListChildContent renders the content of a list item's first child
// inline (without adding list prefix), followed by a newline.
func mdRenderListChildContent(sb *strings.Builder, child *Node, depth int) {
	if child == nil {
		sb.WriteString("\n")
		return
	}
	switch child.Type {
	case TypeParagraph:
		mdRenderInline(sb, child)
		sb.WriteString("\n")
	case TypeBulletList:
		// Nested bullet list directly under list item (no paragraph wrapper)
		sb.WriteString("\n")
		mdRenderNode(sb, child, depth+1)
	case TypeOrderedList:
		sb.WriteString("\n")
		mdRenderNode(sb, child, depth+1)
	default:
		mdRenderNode(sb, child, depth)
		sb.WriteString("\n")
	}
}

// mdRenderBlockquoteChild renders a child of a blockquote with "> " prefix.
func mdRenderBlockquoteChild(sb *strings.Builder, child *Node) {
	if child == nil {
		return
	}
	switch child.Type {
	case TypeParagraph:
		sb.WriteString("> ")
		mdRenderInline(sb, child)
		sb.WriteString("\n")
	default:
		// For other block types inside blockquotes, render and prefix each line
		var inner strings.Builder
		mdRenderNode(&inner, child, 0)
		lines := strings.Split(strings.TrimRight(inner.String(), "\n"), "\n")
		for _, line := range lines {
			sb.WriteString("> ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}
}

// mdRenderTable renders an ADF table as a GFM Markdown table.
func mdRenderTable(sb *strings.Builder, n *Node) {
	if len(n.Content) == 0 {
		return
	}

	for i, row := range n.Content {
		if row.Type != TypeTableRow {
			continue
		}
		sb.WriteString("|")
		for _, cell := range row.Content {
			sb.WriteString(" ")
			mdRenderCellInline(sb, cell)
			sb.WriteString(" |")
		}
		sb.WriteString("\n")

		// After the first row (header), write the separator line
		if i == 0 {
			sb.WriteString("|")
			for range row.Content {
				sb.WriteString("---|")
			}
			sb.WriteString("\n")
		}
	}
}

// mdRenderCellInline renders the inline content of a table cell.
// Cells contain block nodes (typically paragraphs); we extract inline text.
func mdRenderCellInline(sb *strings.Builder, cell *Node) {
	for _, child := range cell.Content {
		if child.Type == TypeParagraph {
			mdRenderInline(sb, child)
		} else {
			mdRenderFallbackText(sb, child)
		}
	}
}

// mdRenderInline renders all inline children of a block node.
func mdRenderInline(sb *strings.Builder, n *Node) {
	for _, child := range n.Content {
		switch child.Type {
		case TypeText:
			sb.WriteString(mdRenderTextWithMarks(child))
		case TypeHardBreak:
			sb.WriteString("\n")
		default:
			// Recurse for any unexpected inline structure
			mdRenderInline(sb, child)
		}
	}
}

// mdRenderTextWithMarks applies inline Markdown marks to a text node.
func mdRenderTextWithMarks(n *Node) string {
	text := n.Text
	for _, m := range n.Marks {
		switch m.Type {
		case MarkStrong:
			text = "**" + text + "**"
		case MarkEm:
			text = "*" + text + "*"
		case MarkCode:
			text = "`" + text + "`"
		case MarkLink:
			if href, ok := m.Attrs["href"].(string); ok {
				text = "[" + text + "](" + href + ")"
			}
		case MarkStrike:
			text = "~~" + text + "~~"
		}
	}
	return text
}

// mdRenderFallbackText extracts and concatenates all text content from an
// unknown node type, matching ExtractText behavior.
func mdRenderFallbackText(sb *strings.Builder, n *Node) {
	if n == nil {
		return
	}
	if n.Type == TypeText {
		sb.WriteString(n.Text)
		return
	}
	for _, child := range n.Content {
		mdRenderFallbackText(sb, child)
	}
}
