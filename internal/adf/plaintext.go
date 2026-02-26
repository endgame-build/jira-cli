package adf

import (
	"encoding/json"
	"strings"
)

// ExtractText walks an ADF document (as raw JSON) and returns a plain-text
// representation by concatenating all text nodes. Block-level nodes are
// separated by newlines. Returns an empty string for nil/empty input.
func ExtractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	var doc Node
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ""
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
