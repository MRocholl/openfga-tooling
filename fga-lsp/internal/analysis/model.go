package analysis

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
	ts "github.com/tree-sitter/go-tree-sitter"
)

// RefKind classifies a name used inside a relation definition. Resolving one
// needs to know what it is looking for: `viewer` after `define x:` is a
// relation on the enclosing type, the same word inside `[...]` is a type.
type RefKind int

const (
	RefType RefKind = iota
	// RefRelationOnSelf is a relation of the enclosing type: the `viewer` in
	// `define x: viewer`, and the `parent` in `viewer from parent`.
	RefRelationOnSelf
	// RefRelationViaTupleset is the `viewer` in `viewer from parent`, which
	// lives on whatever types `parent` can point at.
	RefRelationViaTupleset
	// RefRelationOnType is the `member` in `[team#member]`.
	RefRelationOnType
	RefCondition
)

// Ref is one resolvable name inside a relation definition.
type Ref struct {
	Kind RefKind
	Name string
	// OwnerType is set for RefRelationOnType, Tupleset for
	// RefRelationViaTupleset.
	OwnerType string
	Tupleset  string

	Range     protocol.Range
	StartByte uint
	EndByte   uint
}

// RelationDecl is a single `define` line.
type RelationDecl struct {
	Name     string
	TypeName string
	URI      protocol.DocumentUri

	Range     protocol.Range
	NameRange protocol.Range
	Doc       string
	Expr      string

	Refs []Ref
}

// TypeDecl is one `type X` or `extend type X` block. A type may be declared
// once and extended in any number of modules, so a name maps to several of
// these.
type TypeDecl struct {
	Name   string
	Extend bool
	Module string
	URI    protocol.DocumentUri

	Range     protocol.Range
	NameRange protocol.Range
	Doc       string

	Relations []*RelationDecl
}

// ConditionParam is one `name: type` pair in a condition signature.
type ConditionParam struct {
	Name      string
	Type      string
	Range     protocol.Range
	NameRange protocol.Range
}

type ConditionDecl struct {
	Name   string
	Module string
	URI    protocol.DocumentUri

	Range     protocol.Range
	NameRange protocol.Range
	Doc       string
	Params    []ConditionParam
}

// Extract walks a parsed model document and fills in its symbols.
func Extract(doc *Document) {
	doc.Module = ""
	doc.Types = nil
	doc.Conds = nil

	root := doc.Root()
	if root == nil {
		return
	}

	cursor := root.Walk()
	defer cursor.Close()

	for _, child := range root.NamedChildren(cursor) {
		switch child.Kind() {
		case "module_header":
			doc.Module = doc.Text(child.ChildByFieldName("name"))
		case "type_definition":
			doc.Types = append(doc.Types, extractType(doc, &child))
		case "condition":
			doc.Conds = append(doc.Conds, extractCondition(doc, &child))
		}
	}
}

func extractType(doc *Document, n *ts.Node) *TypeDecl {
	name := n.ChildByFieldName("name")

	decl := &TypeDecl{
		Name:      doc.Text(name),
		Extend:    hasAnonChild(n, "extend"),
		Module:    doc.Module,
		URI:       doc.URI,
		Range:     doc.Lines.NodeRange(n),
		NameRange: nodeRangeOr(doc, name, n),
		Doc:       leadingComment(doc, n),
	}

	block := childOfKind(n, "relations_block")
	if block == nil {
		return decl
	}

	cursor := block.Walk()
	defer cursor.Close()

	for _, child := range block.NamedChildren(cursor) {
		if child.Kind() != "relation_definition" {
			continue
		}

		decl.Relations = append(decl.Relations, extractRelation(doc, &child, decl.Name))
	}

	return decl
}

func extractRelation(doc *Document, n *ts.Node, typeName string) *RelationDecl {
	name := n.ChildByFieldName("name")
	value := n.ChildByFieldName("value")

	rel := &RelationDecl{
		Name:      doc.Text(name),
		TypeName:  typeName,
		URI:       doc.URI,
		Range:     doc.Lines.NodeRange(n),
		NameRange: nodeRangeOr(doc, name, n),
		Doc:       leadingComment(doc, n),
		Expr:      strings.TrimSpace(doc.Text(value)),
	}

	if value != nil {
		collectRefs(doc, value, &rel.Refs)
	}

	return rel
}

// collectRefs walks a relation's right-hand side and records every name that
// points at something else in the model.
func collectRefs(doc *Document, n *ts.Node, out *[]Ref) {
	switch n.Kind() {
	case "computed_relation":
		appendRef(doc, out, Ref{Kind: RefRelationOnSelf}, n.ChildByFieldName("relation"))

		return

	case "tupleset_relation":
		tupleset := n.ChildByFieldName("tupleset")

		appendRef(doc, out, Ref{
			Kind:     RefRelationViaTupleset,
			Tupleset: doc.Text(tupleset),
		}, n.ChildByFieldName("relation"))

		appendRef(doc, out, Ref{Kind: RefRelationOnSelf}, tupleset)

		return

	case "type_restriction":
		typeNode := n.ChildByFieldName("type")
		typeName := doc.Text(typeNode)

		appendRef(doc, out, Ref{Kind: RefType}, typeNode)

		if suffix := childOfKind(n, "relation_suffix"); suffix != nil {
			appendRef(doc, out, Ref{
				Kind:      RefRelationOnType,
				OwnerType: typeName,
			}, suffix.ChildByFieldName("relation"))
		}

		if suffix := childOfKind(n, "condition_suffix"); suffix != nil {
			if cond := suffix.ChildByFieldName("condition"); cond != nil && cond.Kind() == "identifier" {
				appendRef(doc, out, Ref{Kind: RefCondition}, cond)
			}
		}

		return
	}

	cursor := n.Walk()
	defer cursor.Close()

	for _, child := range n.NamedChildren(cursor) {
		collectRefs(doc, &child, out)
	}
}

func appendRef(doc *Document, out *[]Ref, ref Ref, n *ts.Node) {
	if n == nil {
		return
	}

	ref.Name = doc.Text(n)
	ref.Range = doc.Lines.NodeRange(n)
	ref.StartByte, ref.EndByte = n.ByteRange()

	*out = append(*out, ref)
}

func extractCondition(doc *Document, n *ts.Node) *ConditionDecl {
	name := n.ChildByFieldName("name")

	cond := &ConditionDecl{
		Name:      doc.Text(name),
		Module:    doc.Module,
		URI:       doc.URI,
		Range:     doc.Lines.NodeRange(n),
		NameRange: nodeRangeOr(doc, name, n),
		Doc:       leadingComment(doc, n),
	}

	list := childOfKind(n, "parameter_list")
	if list == nil {
		return cond
	}

	cursor := list.Walk()
	defer cursor.Close()

	for _, child := range list.NamedChildren(cursor) {
		if child.Kind() != "parameter" {
			continue
		}

		paramName := child.ChildByFieldName("name")

		cond.Params = append(cond.Params, ConditionParam{
			Name:      doc.Text(paramName),
			Type:      doc.Text(child.ChildByFieldName("type")),
			Range:     doc.Lines.NodeRange(&child),
			NameRange: nodeRangeOr(doc, paramName, &child),
		})
	}

	return cond
}

// leadingComment gathers the run of `#` lines directly above a declaration,
// which is where this codebase puts the rationale worth showing on hover.
func leadingComment(doc *Document, n *ts.Node) string {
	var lines []string

	expected := int(n.StartPosition().Row) - 1

	for prev := n.PrevSibling(); prev != nil; prev = prev.PrevSibling() {
		if prev.Kind() != "comment" {
			break
		}

		row := int(prev.StartPosition().Row)
		if row != expected {
			break
		}

		text := doc.Text(prev)
		text = strings.TrimPrefix(text, "#")
		lines = append([]string{strings.TrimSpace(text)}, lines...)
		expected = row - 1
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func childOfKind(n *ts.Node, kind string) *ts.Node {
	for i := range n.NamedChildCount() {
		if child := n.NamedChild(i); child != nil && child.Kind() == kind {
			return child
		}
	}

	return nil
}

func hasAnonChild(n *ts.Node, kind string) bool {
	for i := range n.ChildCount() {
		if child := n.Child(i); child != nil && child.Kind() == kind {
			return true
		}
	}

	return false
}

func nodeRangeOr(doc *Document, n, fallback *ts.Node) protocol.Range {
	if n == nil {
		return doc.Lines.NodeRange(fallback)
	}

	return doc.Lines.NodeRange(n)
}
