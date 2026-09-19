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
	// RefTupleset is the `parent` in `viewer from parent`: a relation of the
	// enclosing type, but one that has to be directly assignable.
	RefTupleset
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

	// Text and OuterRange describe the whole construct the name sits in --
	// `group#member` rather than `group`, `viewer from parent` rather than
	// `viewer`. Upstream reports several errors against the construct, not
	// the word.
	Text       string
	OuterRange protocol.Range
}

// Partial is one operand of a relation's top-level `or`, `and` or `but not`.
// A relation with no operator has exactly one.
type Partial struct {
	Text  string
	Range protocol.Range
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

	// DirectOnly marks a relation that can legally appear on the right of
	// `from`: defined by a type restriction alone, and restricted to plain
	// types. A userset (`folder#parent`) or a wildcard (`folder:*`) in that
	// list disqualifies it, because a tupleset has to name objects to walk
	// to, not sets of users.
	DirectOnly bool

	Partials []Partial
	Refs     []Ref
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
	doc.Schema = ""
	doc.HasHeader = false
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
		case "model_header":
			doc.HasHeader = true

			if version := child.ChildByFieldName("version"); version != nil {
				doc.Schema = doc.Text(version)
				doc.SchemaRange = doc.Lines.NodeRange(version)
			}
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
		rel.DirectOnly = value.Kind() == "direct_assignment" && onlyPlainTypes(value)
		rel.Partials = collectPartials(doc, value)

		collectRefs(doc, value, &rel.Refs)
	}

	return rel
}

// onlyPlainTypes reports whether every restriction in a direct assignment
// names a bare type.
func onlyPlainTypes(assignment *ts.Node) bool {
	cursor := assignment.Walk()
	defer cursor.Close()

	for _, child := range assignment.NamedChildren(cursor) {
		if child.Kind() != "type_restriction" {
			continue
		}

		if childOfKind(&child, "wildcard") != nil || childOfKind(&child, "relation_suffix") != nil {
			return false
		}
	}

	return true
}

// collectPartials splits a relation's right-hand side at its top-level
// operator. `a or b or c` yields three; `a` yields one.
func collectPartials(doc *Document, value *ts.Node) []Partial {
	switch value.Kind() {
	case "union", "intersection", "exclusion":
		cursor := value.Walk()
		defer cursor.Close()

		var out []Partial

		for _, child := range value.NamedChildren(cursor) {
			if child.Kind() == "but_not" || child.Kind() == "comment" {
				continue
			}

			out = append(out, Partial{
				Text:  strings.TrimSpace(doc.Text(&child)),
				Range: doc.Lines.NodeRange(&child),
			})
		}

		return out
	}

	return []Partial{{
		Text:  strings.TrimSpace(doc.Text(value)),
		Range: doc.Lines.NodeRange(value),
	}}
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
		outer := doc.Lines.NodeRange(n)
		text := doc.Text(n)

		appendRef(doc, out, Ref{
			Kind:       RefRelationViaTupleset,
			Tupleset:   doc.Text(tupleset),
			Text:       text,
			OuterRange: outer,
		}, n.ChildByFieldName("relation"))

		appendRef(doc, out, Ref{
			Kind:       RefTupleset,
			Text:       text,
			OuterRange: outer,
		}, tupleset)

		return

	case "type_restriction":
		typeNode := n.ChildByFieldName("type")
		typeName := doc.Text(typeNode)
		outer := doc.Lines.NodeRange(n)
		text := doc.Text(n)

		appendRef(doc, out, Ref{Kind: RefType, Text: text, OuterRange: outer}, typeNode)

		if suffix := childOfKind(n, "relation_suffix"); suffix != nil {
			appendRef(doc, out, Ref{
				Kind:       RefRelationOnType,
				OwnerType:  typeName,
				Text:       text,
				OuterRange: outer,
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

	if ref.Text == "" {
		ref.Text = ref.Name
		ref.OuterRange = ref.Range
	}

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

		// A comment trailing code on the line above documents that line, not
		// this one. Only a comment that opens its line is a doc comment.
		if !opensLine(doc, prev) {
			break
		}

		text := doc.Text(prev)
		text = strings.TrimPrefix(text, "#")
		lines = append([]string{strings.TrimSpace(text)}, lines...)
		expected = row - 1
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// opensLine reports whether n is the first non-space thing on its line.
func opensLine(doc *Document, n *ts.Node) bool {
	line := doc.Lines.LineBytes(int(n.StartPosition().Row))

	column := int(n.StartPosition().Column)
	if column > len(line) {
		return false
	}

	for _, b := range line[:column] {
		if b != ' ' && b != '\t' {
			return false
		}
	}

	return true
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
