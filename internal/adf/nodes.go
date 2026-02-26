// Package adf provides types and conversion utilities for Atlassian Document Format (ADF).
// ADF is the mandatory rich-text format for Jira Cloud v3 description and comment fields.
package adf

// NodeType identifies the type of an ADF node.
type NodeType string

const (
	TypeDoc         NodeType = "doc"
	TypeParagraph   NodeType = "paragraph"
	TypeHeading     NodeType = "heading"
	TypeBulletList  NodeType = "bulletList"
	TypeOrderedList NodeType = "orderedList"
	TypeListItem    NodeType = "listItem"
	TypeCodeBlock   NodeType = "codeBlock"
	TypeBlockquote  NodeType = "blockquote"
	TypeRule        NodeType = "rule"
	TypeText        NodeType = "text"
	TypeHardBreak   NodeType = "hardBreak"
)

// MarkType identifies the type of an inline mark.
type MarkType string

const (
	MarkStrong MarkType = "strong"
	MarkEm     MarkType = "em"
	MarkCode   MarkType = "code"
	MarkLink   MarkType = "link"
	MarkStrike MarkType = "strike"
)

// Node is the building block of an ADF document tree.
type Node struct {
	Type    NodeType               `json:"type"`
	Version int                    `json:"version,omitempty"` // only on doc node (always 1)
	Content []*Node                `json:"content,omitempty"`
	Text    string                 `json:"text,omitempty"`
	Marks   []Mark                 `json:"marks,omitempty"`
	Attrs   map[string]interface{} `json:"attrs,omitempty"`
}

// Mark represents an inline formatting mark on a text node.
type Mark struct {
	Type  MarkType               `json:"type"`
	Attrs map[string]interface{} `json:"attrs,omitempty"`
}

// Document creates a new ADF document root node.
func Document(content ...*Node) *Node {
	return &Node{
		Type:    TypeDoc,
		Version: 1,
		Content: content,
	}
}

// Paragraph creates a paragraph node.
func Paragraph(content ...*Node) *Node {
	return &Node{
		Type:    TypeParagraph,
		Content: content,
	}
}

// Heading creates a heading node with the given level (1-6).
func Heading(level int, content ...*Node) *Node {
	return &Node{
		Type:    TypeHeading,
		Attrs:   map[string]interface{}{"level": level},
		Content: content,
	}
}

// BulletList creates a bullet list node.
func BulletList(items ...*Node) *Node {
	return &Node{
		Type:    TypeBulletList,
		Content: items,
	}
}

// OrderedList creates an ordered list node.
func OrderedList(items ...*Node) *Node {
	return &Node{
		Type:    TypeOrderedList,
		Content: items,
	}
}

// ListItem creates a list item node.
func ListItem(content ...*Node) *Node {
	return &Node{
		Type:    TypeListItem,
		Content: content,
	}
}

// CodeBlock creates a code block node with an optional language attribute.
func CodeBlock(language string, content ...*Node) *Node {
	n := &Node{
		Type:    TypeCodeBlock,
		Content: content,
	}
	if language != "" {
		n.Attrs = map[string]interface{}{"language": language}
	}
	return n
}

// Blockquote creates a blockquote node.
func Blockquote(content ...*Node) *Node {
	return &Node{
		Type:    TypeBlockquote,
		Content: content,
	}
}

// Rule creates a horizontal rule node.
func Rule() *Node {
	return &Node{Type: TypeRule}
}

// Text creates a text node with optional marks.
func Text(text string, marks ...Mark) *Node {
	n := &Node{
		Type: TypeText,
		Text: text,
	}
	if len(marks) > 0 {
		n.Marks = marks
	}
	return n
}

// HardBreak creates a hard break (line break) node.
func HardBreak() *Node {
	return &Node{Type: TypeHardBreak}
}

// Strong creates a strong (bold) mark.
func Strong() Mark {
	return Mark{Type: MarkStrong}
}

// Em creates an emphasis (italic) mark.
func Em() Mark {
	return Mark{Type: MarkEm}
}

// Code creates an inline code mark.
func Code() Mark {
	return Mark{Type: MarkCode}
}

// Link creates a link mark with the given href.
func Link(href string) Mark {
	return Mark{
		Type:  MarkLink,
		Attrs: map[string]interface{}{"href": href},
	}
}

// Strike creates a strikethrough mark.
func Strike() Mark {
	return Mark{Type: MarkStrike}
}
