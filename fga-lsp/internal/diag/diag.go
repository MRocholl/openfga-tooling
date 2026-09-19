// Package diag turns the state of a workspace into LSP diagnostics.
//
// Three passes, cheapest first, each gating the next:
//
//  1. the tree-sitter tree, which has an answer for a half-typed buffer;
//  2. the reference transformer, which decides whether the DSL is legal;
//  3. openfga's own typesystem, the same check `fga model validate` runs.
//
// Passes 2 and 3 answer for a whole module set at once, so the result is
// keyed by document: editing one module reports errors in its siblings.
package diag

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/openfga/openfga/pkg/typesystem"
	protocol "github.com/tliron/glsp/protocol_3_16"
	ts "github.com/tree-sitter/go-tree-sitter"

	"github.com/mrocholl/fga-lsp/internal/analysis"
)

const source = "fga"

// Result maps each affected document to its complete diagnostic list. A
// document present with an empty slice has its diagnostics cleared.
type Result map[protocol.DocumentUri][]protocol.Diagnostic

func (r Result) add(uri protocol.DocumentUri, d protocol.Diagnostic) {
	r[uri] = append(r[uri], d)
}

func (r Result) touch(uri protocol.DocumentUri) {
	if _, ok := r[uri]; !ok {
		r[uri] = []protocol.Diagnostic{}
	}
}

// Compute produces diagnostics for doc and every document that shares its
// module set.
func Compute(view *analysis.View, doc *analysis.Document) Result {
	result := Result{}
	result.touch(doc.URI)

	switch doc.Kind {
	case analysis.KindModel:
		modelDiagnostics(view, doc, result)
	case analysis.KindStoreTest:
		storeDiagnostics(view, doc, result)
	case analysis.KindModFile:
		modDiagnostics(view, doc, result)
	case analysis.KindUnknown:
	}

	return result
}

// ------------------------------------------------------------------- models

func modelDiagnostics(view *analysis.View, doc *analysis.Document, result Result) {
	set := moduleSet(view, doc)
	for _, member := range set.docs {
		result.touch(member.URI)
	}

	syntaxBroken := false

	for _, member := range set.docs {
		// tree-sitter first: it has an answer for a half-typed buffer, and
		// its ranges are tighter. ANTLR only runs when that pass is happy,
		// to catch what the looser grammar lets through without reporting
		// the same break twice.
		if syntaxErrors(member, result) || antlrSyntaxErrors(member, result) {
			syntaxBroken = true
		}
	}

	// Name resolution runs even when the DSL does not compile: an unknown
	// relation is exactly what a half-finished edit produces, and pointing at
	// it is more useful than waiting for the file to become valid.
	names := scopeOf(view, set.scoped())

	// Names that resolution has already reported. The typesystem repeats them
	// in its own words, and having no source position for an undefined
	// relation, would pin the repeat to the top of the file.
	covered := map[string]struct{}{}

	for _, member := range set.docs {
		semanticErrors(names, member, set.mod != nil, result, covered)
	}

	if syntaxBroken {
		return
	}

	model, buildErrs := set.build(view)
	for _, problem := range buildErrs {
		result.add(problem.uri, diagnostic(problem.rng, problem.message, protocol.DiagnosticSeverityError))
	}

	if model == nil {
		return
	}

	if _, err := typesystem.NewAndValidate(context.Background(), model); err != nil {
		reportTypesystem(view, set, err, covered, result)
	}
}

// syntaxErrors walks the parse tree for error and missing nodes. It reports
// whether any were found.
func syntaxErrors(doc *analysis.Document, result Result) bool {
	root := doc.Root()
	if root == nil || !root.HasError() {
		return false
	}

	found := false

	walk(root, func(n *ts.Node) bool {
		switch {
		case n.IsMissing():
			found = true

			result.add(doc.URI, diagnostic(
				doc.Lines.NodeRange(n),
				fmt.Sprintf("missing %s", n.Kind()),
				protocol.DiagnosticSeverityError,
			))

			return false

		case n.IsError():
			found = true

			text := strings.TrimSpace(doc.Text(n))
			message := "unexpected input"

			if text != "" {
				message = fmt.Sprintf("unexpected %q", firstLine(text))
			}

			result.add(doc.URI, diagnostic(
				doc.Lines.NodeRange(n),
				message,
				protocol.DiagnosticSeverityError,
			))

			return false
		}

		return n.HasError()
	})

	return found
}

// reportTypesystem maps an openfga validation failure onto the declaration it
// names. Anything already reported by name resolution is dropped, since that
// pass underlines the offending word rather than the whole line.
func reportTypesystem(
	view *analysis.View,
	set moduleSetInfo,
	err error,
	covered map[string]struct{},
	result Result,
) {
	for _, single := range flatten(err) {
		objectType, relation := subject(single)

		if _, ok := covered[subjectKey(objectType, relation)]; ok {
			continue
		}

		var (
			uri protocol.DocumentUri
			rng protocol.Range
			ok  bool
		)

		switch {
		case objectType != "" && relation != "":
			uri, rng, ok = relationRange(view, set, objectType, relation)
		case objectType != "":
			uri, rng, ok = typeRange(view, set, objectType)
		case relation != "":
			uri, rng, ok = anyRelationRange(view, set, relation)
		}

		if !ok {
			uri, rng = set.fallback()
		}

		message := single.Error()
		if duplicate(result[uri], rng, message) {
			continue
		}

		result.add(uri, diagnostic(rng, message, protocol.DiagnosticSeverityError))
	}
}

// messageSubjects recover a type and relation from an error that carries no
// structured fields. Most of the typesystem's failures are plain fmt.Errorf
// values, and without this every one of them lands on line 1 of the file.
var messageSubjects = []*regexp.Regexp{
	regexp.MustCompile(`relation '([^']+)' in object type '([^']+)'`),
	regexp.MustCompile(`on '([^']+)' in object type '([^']+)'`),
	regexp.MustCompile(`'([^'#]+)#([^']+)' relation`),
}

func subject(err error) (objectType, relation string) {
	var (
		invalidType   *typesystem.InvalidTypeError
		invalidRel    *typesystem.InvalidRelationError
		undefinedType *typesystem.ObjectTypeUndefinedError
		undefinedRel  *typesystem.RelationUndefinedError
		condition     *typesystem.RelationConditionError
	)

	switch {
	case errors.As(err, &invalidRel):
		return invalidRel.ObjectType, invalidRel.Relation
	case errors.As(err, &undefinedRel):
		return undefinedRel.ObjectType, undefinedRel.Relation
	case errors.As(err, &invalidType):
		return invalidType.ObjectType, ""
	case errors.As(err, &undefinedType):
		return undefinedType.ObjectType, ""
	case errors.As(err, &condition):
		return "", condition.Relation
	}

	return subjectFromMessage(err.Error())
}

func subjectFromMessage(message string) (objectType, relation string) {
	for i, pattern := range messageSubjects {
		match := pattern.FindStringSubmatch(message)
		if match == nil {
			continue
		}

		// The last pattern reads `type#relation`; the others name the
		// relation first.
		if i == len(messageSubjects)-1 {
			return match[1], match[2]
		}

		return match[2], match[1]
	}

	if match := regexp.MustCompile(`undefined relation: ([\w./-]+)`).FindStringSubmatch(message); match != nil {
		return "", match[1]
	}

	return "", ""
}

// anyRelationRange places an error that names a relation but not its type.
func anyRelationRange(
	view *analysis.View,
	set moduleSetInfo,
	relation string,
) (protocol.DocumentUri, protocol.Range, bool) {
	for _, doc := range set.docs {
		for _, decl := range doc.Types {
			for _, rel := range decl.Relations {
				if rel.Name == relation {
					return doc.URI, rel.NameRange, true
				}
			}
		}
	}

	return "", protocol.Range{}, false
}

func relationRange(
	view *analysis.View,
	set moduleSetInfo,
	objectType, relation string,
) (protocol.DocumentUri, protocol.Range, bool) {
	for _, decl := range view.RelationDecls(objectType, relation) {
		if set.contains(decl.URI) {
			return decl.URI, decl.NameRange, true
		}
	}

	return "", protocol.Range{}, false
}

func typeRange(
	view *analysis.View,
	set moduleSetInfo,
	objectType string,
) (protocol.DocumentUri, protocol.Range, bool) {
	for _, decl := range view.TypeDecls(objectType) {
		if set.contains(decl.URI) {
			return decl.URI, decl.NameRange, true
		}
	}

	return "", protocol.Range{}, false
}

// ------------------------------------------------------------------ helpers

func diagnostic(rng protocol.Range, message string, severity protocol.DiagnosticSeverity) protocol.Diagnostic {
	src := source

	return protocol.Diagnostic{
		Range:    rng,
		Severity: &severity,
		Source:   &src,
		Message:  message,
	}
}

func duplicate(existing []protocol.Diagnostic, rng protocol.Range, message string) bool {
	for _, d := range existing {
		if d.Range == rng && d.Message == message {
			return true
		}
	}

	return false
}

// flatten unwraps the multi-error wrappers both libraries use.
func flatten(err error) []error {
	if err == nil {
		return nil
	}

	var multi interface{ Unwrap() []error }
	if errors.As(err, &multi) {
		var out []error
		for _, inner := range multi.Unwrap() {
			out = append(out, flatten(inner)...)
		}

		if len(out) > 0 {
			return out
		}
	}

	return []error{err}
}

func walk(n *ts.Node, visit func(*ts.Node) bool) {
	if !visit(n) {
		return
	}

	for i := range n.ChildCount() {
		if child := n.Child(i); child != nil {
			walk(child, visit)
		}
	}
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}

	// Truncating by byte can split a rune and put invalid UTF-8 on the wire.
	const limit = 40

	if utf8.RuneCountInString(s) <= limit {
		return s
	}

	return string([]rune(s)[:limit]) + "..."
}
