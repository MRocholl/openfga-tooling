package feature

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/analysis"
)

const (
	indentType     = ""
	indentRelation = "  "
	indentDefine   = "    "
)

// Format re-renders a model file from its parse tree.
func Format(doc *analysis.Document) []protocol.TextEdit {
	root := doc.Root()
	if root == nil || root.HasError() {
		return nil
	}

	formatted := renderFile(doc, root)

	if formatted == string(doc.Content) {
		return nil
	}

	return []protocol.TextEdit{{
		Range: protocol.Range{
			Start: protocol.Position{},
			End:   doc.Lines.LineRange(doc.Lines.LineCount() - 1).End,
		},
		NewText: formatted,
	}}
}

func renderFile(doc *analysis.Document, root *ts.Node) string {
	var b strings.Builder

	cursor := root.Walk()
	defer cursor.Close()

	previousEnd := -1

	for _, child := range root.NamedChildren(cursor) {
		start := int(child.StartPosition().Row)

		if previousEnd >= 0 && start > previousEnd+1 {
			b.WriteString("\n")
		}

		b.WriteString(renderTopLevel(doc, &child))

		previousEnd = int(child.EndPosition().Row)
	}

	return b.String()
}

func renderTopLevel(doc *analysis.Document, n *ts.Node) string {
	switch n.Kind() {
	case "comment":
		return comment(doc, n) + "\n"

	case "model_header":
		version := doc.Text(n.ChildByFieldName("version"))

		return "model\n  schema " + version + "\n"

	case "module_header":
		return "module " + doc.Text(n.ChildByFieldName("name")) + "\n"

	case "type_definition":
		return renderType(doc, n)

	case "condition":
		return renderCondition(doc, n)
	}

	return doc.Text(n) + "\n"
}

func renderType(doc *analysis.Document, n *ts.Node) string {
	var b strings.Builder

	b.WriteString(indentType)

	if hasChild(n, "extend") {
		b.WriteString("extend ")
	}

	b.WriteString("type " + doc.Text(n.ChildByFieldName("name")) + "\n")

	block := namedChild(n, "relations_block")
	if block == nil {
		return b.String()
	}

	b.WriteString(indentRelation + "relations\n")

	cursor := block.Walk()
	defer cursor.Close()

	previousEnd := -1

	for _, child := range block.NamedChildren(cursor) {
		// Blank lines inside a relations block separate the capabilities from
		// the permissions derived off them. That grouping is the author's, so
		// it survives here exactly as it does between top-level declarations.
		start := int(child.StartPosition().Row)
		if previousEnd >= 0 && start > previousEnd+1 {
			b.WriteString("\n")
		}

		switch child.Kind() {
		case "comment":
			b.WriteString(indentDefine + comment(doc, &child) + "\n")
		case "relation_definition":
			b.WriteString(indentDefine + renderRelation(doc, &child) + "\n")
		}

		previousEnd = int(child.EndPosition().Row)
	}

	return b.String()
}

func renderRelation(doc *analysis.Document, n *ts.Node) string {
	name := doc.Text(n.ChildByFieldName("name"))
	value := renderExpr(doc, n.ChildByFieldName("value"))

	return "define " + name + ": " + value
}

func renderExpr(doc *analysis.Document, n *ts.Node) string {
	if n == nil {
		return ""
	}

	cursor := n.Walk()
	defer cursor.Close()

	switch n.Kind() {
	case "union":
		return joinOperands(doc, n, " or ")

	case "intersection":
		return joinOperands(doc, n, " and ")

	case "exclusion":
		parts := operands(doc, n)

		return strings.Join(parts, " but not ")

	case "grouping":
		return "(" + renderExpr(doc, n.NamedChild(0)) + ")"

	case "computed_relation":
		return doc.Text(n.ChildByFieldName("relation"))

	case "tupleset_relation":
		return doc.Text(n.ChildByFieldName("relation")) + " from " + doc.Text(n.ChildByFieldName("tupleset"))

	case "direct_assignment":
		restrictions := make([]string, 0, n.NamedChildCount())
		for _, child := range n.NamedChildren(cursor) {
			if child.Kind() == "type_restriction" {
				restrictions = append(restrictions, renderRestriction(doc, &child))
			}
		}

		return "[" + strings.Join(restrictions, ", ") + "]"

	case "type_restriction":
		return renderRestriction(doc, n)
	}

	return strings.TrimSpace(doc.Text(n))
}

// operands collects the parts of a binary form, skipping the `but not` token which is a named node so that...
func operands(doc *analysis.Document, n *ts.Node) []string {
	cursor := n.Walk()
	defer cursor.Close()

	var parts []string

	for _, child := range n.NamedChildren(cursor) {
		if child.Kind() == "but_not" || child.Kind() == "comment" {
			continue
		}

		parts = append(parts, renderExpr(doc, &child))
	}

	return parts
}

func joinOperands(doc *analysis.Document, n *ts.Node, sep string) string {
	return strings.Join(operands(doc, n), sep)
}

func renderRestriction(doc *analysis.Document, n *ts.Node) string {
	var b strings.Builder

	b.WriteString(doc.Text(n.ChildByFieldName("type")))

	if namedChild(n, "wildcard") != nil {
		b.WriteString(":*")
	}

	if suffix := namedChild(n, "relation_suffix"); suffix != nil {
		b.WriteString("#" + doc.Text(suffix.ChildByFieldName("relation")))
	}

	if suffix := namedChild(n, "condition_suffix"); suffix != nil {
		b.WriteString(" with " + doc.Text(suffix.ChildByFieldName("condition")))
	}

	return b.String()
}

func renderCondition(doc *analysis.Document, n *ts.Node) string {
	var b strings.Builder

	b.WriteString("condition " + doc.Text(n.ChildByFieldName("name")) + "(")

	list := namedChild(n, "parameter_list")
	if list != nil {
		cursor := list.Walk()
		defer cursor.Close()

		params := make([]string, 0, list.NamedChildCount())

		for _, child := range list.NamedChildren(cursor) {
			if child.Kind() != "parameter" {
				continue
			}

			params = append(params,
				doc.Text(child.ChildByFieldName("name"))+": "+doc.Text(child.ChildByFieldName("type")))
		}

		b.WriteString(strings.Join(params, ", "))
	}

	b.WriteString(") {\n")

	body := n.ChildByFieldName("body")
	if body != nil {
		b.WriteString(renderConditionBody(doc, body))
	}

	b.WriteString("}\n")

	return b.String()
}

// renderConditionBody re-indents the CEL source without reformatting it.
func renderConditionBody(doc *analysis.Document, body *ts.Node) string {
	text := doc.Text(body)
	text = strings.TrimPrefix(text, "{")
	text = strings.TrimSuffix(text, "}")

	lines := strings.Split(strings.Trim(text, "\n"), "\n")

	common := commonIndent(lines)

	var b strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimRight(strings.TrimPrefix(line, common), " \t")
		if strings.TrimSpace(trimmed) == "" {
			b.WriteString("\n")

			continue
		}

		b.WriteString(indentRelation + trimmed + "\n")
	}

	return b.String()
}

// commonIndent is the whitespace every non-blank line starts with, which is the indentation the body sat at...
func commonIndent(lines []string) string {
	common := ""
	first := true

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]

		if first {
			common, first = indent, false

			continue
		}

		for !strings.HasPrefix(indent, common) {
			common = common[:len(common)-1]
		}
	}

	return common
}

func comment(doc *analysis.Document, n *ts.Node) string {
	return strings.TrimRight(doc.Text(n), " \t")
}

func namedChild(n *ts.Node, kind string) *ts.Node {
	for i := range n.NamedChildCount() {
		if child := n.NamedChild(i); child != nil && child.Kind() == kind {
			return child
		}
	}

	return nil
}

func hasChild(n *ts.Node, kind string) bool {
	for i := range n.ChildCount() {
		if child := n.Child(i); child != nil && child.Kind() == kind {
			return true
		}
	}

	return false
}
