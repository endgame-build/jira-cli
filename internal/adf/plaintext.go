package adf

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ExtractText walks an ADF document (as raw JSON) and returns a plain-text
// representation by concatenating all text nodes. Block-level nodes are
// separated by newlines. Returns an empty string for nil/empty input.
// Returns the raw string as fallback for invalid JSON.
func ExtractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var doc Node
	if err := json.Unmarshal(raw, &doc); err != nil {
		return string(raw)
	}

	var sb strings.Builder
	extractNode(&sb, &doc, true)
	return strings.TrimSpace(sb.String())
}

// extractNode recursively walks the ADF tree, appending text to the builder.
// isFirst tracks whether this is the first block node (to avoid leading newlines).
func extractNode(sb *strings.Builder, n *Node, isFirst bool) {
	if n == nil {
		return
	}

	switch n.Type {
	case TypeText:
		sb.WriteString(n.Text)
		return
	case TypeHardBreak:
		sb.WriteString("\n")
		return
	}

	// Block-level nodes get a newline separator (except the first).
	isBlock := n.Type == TypeParagraph || n.Type == TypeHeading ||
		n.Type == TypeCodeBlock || n.Type == TypeBlockquote ||
		n.Type == TypeBulletList || n.Type == TypeOrderedList ||
		n.Type == TypeListItem || n.Type == TypeRule

	if isBlock && !isFirst && sb.Len() > 0 {
		sb.WriteString("\n")
	}

	for i, child := range n.Content {
		extractNode(sb, child, isFirst && i == 0)
	}
}

// ToPlaintext converts an ADF document (as raw JSON) into structured readable
// plaintext suitable for terminal display. Unlike ExtractText, it preserves
// formatting structure: bullets, numbered lists, code block indentation,
// blockquote prefixes, and nested list indentation.
//
// Returns empty string for nil/empty input. Returns the raw string as fallback
// for invalid JSON.
func ToPlaintext(doc json.RawMessage) string {
	if len(doc) == 0 {
		return ""
	}

	var root Node
	if err := json.Unmarshal(doc, &root); err != nil {
		return string(doc)
	}

	var sb strings.Builder
	renderNode(&sb, &root, 0, "", false)
	return strings.TrimRight(sb.String(), "\n")
}

// renderNode recursively walks the ADF tree, producing structured plaintext.
// depth tracks nesting level for indentation, listPrefix is the prefix for
// current list items (e.g., "- " or "1. "), and inBlockquote tracks blockquote
// context for child rendering.
func renderNode(sb *strings.Builder, n *Node, depth int, listPrefix string, inBlockquote bool) {
	if n == nil {
		return
	}

	indent := strings.Repeat("  ", depth)
	bqPrefix := ""
	if inBlockquote {
		bqPrefix = "> "
	}

	switch n.Type {
	case TypeDoc:
		for i, child := range n.Content {
			if i > 0 {
				sb.WriteString("\n")
			}
			renderNode(sb, child, depth, "", false)
		}

	case TypeParagraph:
		sb.WriteString(bqPrefix)
		sb.WriteString(indent)
		sb.WriteString(listPrefix)
		renderInline(sb, n)
		sb.WriteString("\n")

	case TypeHeading:
		sb.WriteString(indent)
		renderInline(sb, n)
		sb.WriteString("\n")

	case TypeBulletList:
		for _, child := range n.Content {
			renderNode(sb, child, depth, "- ", inBlockquote)
		}

	case TypeOrderedList:
		for i, child := range n.Content {
			prefix := fmt.Sprintf("%d. ", i+1)
			renderNode(sb, child, depth, prefix, inBlockquote)
		}

	case TypeListItem:
		// A list item contains block-level content (usually paragraphs).
		// The first child gets the list prefix; subsequent children are continuation lines.
		for i, child := range n.Content {
			if i == 0 {
				renderNode(sb, child, depth, listPrefix, inBlockquote)
			} else {
				// Nested lists or continuation paragraphs indent one level deeper
				renderNode(sb, child, depth+1, "", inBlockquote)
			}
		}

	case TypeCodeBlock:
		for _, child := range n.Content {
			if child.Type == TypeText {
				lines := strings.Split(child.Text, "\n")
				for _, line := range lines {
					sb.WriteString(indent)
					sb.WriteString("    ")
					sb.WriteString(line)
					sb.WriteString("\n")
				}
			}
		}

	case TypeBlockquote:
		for i, child := range n.Content {
			if i > 0 {
				sb.WriteString("\n")
			}
			renderNode(sb, child, depth, "", true)
		}

	case TypeRule:
		sb.WriteString(indent)
		sb.WriteString("---\n")

	case TypeText:
		sb.WriteString(renderTextWithMarks(n))

	case TypeHardBreak:
		sb.WriteString("\n")
	}
}

// renderInline renders all inline children of a block node.
func renderInline(sb *strings.Builder, n *Node) {
	for _, child := range n.Content {
		switch child.Type {
		case TypeText:
			sb.WriteString(renderTextWithMarks(child))
		case TypeHardBreak:
			sb.WriteString("\n")
		default:
			// Recurse for any unexpected inline structure
			renderInline(sb, child)
		}
	}
}

// renderTextWithMarks applies inline marks to a text node.
func renderTextWithMarks(n *Node) string {
	text := n.Text
	for _, m := range n.Marks {
		switch m.Type {
		case MarkCode:
			text = "`" + text + "`"
		case MarkLink:
			if href, ok := m.Attrs["href"].(string); ok {
				text = text + " (" + href + ")"
			}
		case MarkStrong, MarkEm, MarkStrike:
			// stripped — text only for terminal display
		}
	}
	return text
}
