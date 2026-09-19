package feature

import (
	"regexp"
	"sort"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/mrocholl/fga-lsp/internal/analysis"
)

// Completion offers what can legally follow the cursor.
//
// The context is read from the text to the left of the cursor rather than
// from the parse tree. Completion is asked for mid-token, when the buffer is
// by definition not a valid model, and a half-typed `define viewer: [us`
// recovers into a shape the tree cannot describe but the line can.
func Completion(v *analysis.View, doc *analysis.Document, pos protocol.Position) []protocol.CompletionItem {
	prefix := linePrefix(doc, pos)

	switch doc.Kind {
	case analysis.KindModel:
		return modelCompletion(analysis.ScopeFor(v, doc), doc, pos, prefix)
	case analysis.KindStoreTest:
		return storeCompletion(analysis.ScopeFor(v, doc), doc, pos, prefix)
	case analysis.KindModFile, analysis.KindUnknown:
	}

	return nil
}

func linePrefix(doc *analysis.Document, pos protocol.Position) string {
	offset := doc.Lines.PositionToByte(pos)
	if offset > uint(len(doc.Content)) {
		offset = uint(len(doc.Content))
	}

	line := doc.Lines.LineBytes(int(pos.Line))

	start := doc.Lines.PositionToByte(protocol.Position{Line: pos.Line})
	if offset < start {
		return ""
	}

	within := int(offset - start)
	if within > len(line) {
		within = len(line)
	}

	return string(line[:within])
}

var (
	openBracket     = regexp.MustCompile(`\[[^\]]*$`)
	afterWith       = regexp.MustCompile(`\bwith\s+[\w-]*$`)
	afterHash       = regexp.MustCompile(`([\w./-]+)#[\w-]*$`)
	afterFrom       = regexp.MustCompile(`\bfrom\s+[\w-]*$`)
	afterOperator   = regexp.MustCompile(`(:|\bor\b|\band\b|\bbut\s+not\b|\()\s*[\w-]*$`)
	relationRHS     = regexp.MustCompile(`^\s*define\s+[\w./-]+\s*:`)
	afterCompleteID = regexp.MustCompile(`[\w./-]\s+[\w-]*$`)
)

func modelCompletion(
	scope analysis.Scope,
	doc *analysis.Document,
	pos protocol.Position,
	prefix string,
) []protocol.CompletionItem {
	enclosing := enclosingType(doc, pos)

	if openBracket.MatchString(prefix) {
		switch {
		case afterWith.MatchString(prefix):
			return conditionItems(scope)
		case afterHash.MatchString(prefix):
			owner := afterHash.FindStringSubmatch(prefix)[1]

			return relationItems(scope, owner)
		default:
			return typeItems(scope)
		}
	}

	if relationRHS.MatchString(prefix) {
		if enclosing == "" {
			return nil
		}

		switch {
		case afterFrom.MatchString(prefix):
			return relationItems(scope, enclosing)
		case afterOperator.MatchString(prefix):
			items := relationItems(scope, enclosing)

			return append(items, keyword("[", "type restriction", "[$1]"))
		case afterCompleteID.MatchString(prefix):
			return operatorItems()
		}

		return relationItems(scope, enclosing)
	}

	if enclosing != "" && strings.TrimSpace(prefix) == strings.TrimSpace(trimWord(prefix)) {
		return []protocol.CompletionItem{keyword("define", "relation definition", "define ${1:name}: $0")}
	}

	return declarationItems()
}

// enclosingType finds the type whose block the cursor sits in.
func enclosingType(doc *analysis.Document, pos protocol.Position) string {
	best := ""

	for _, decl := range doc.Types {
		if decl.Range.Start.Line <= pos.Line && pos.Line <= decl.Range.End.Line+1 {
			best = decl.Name
		}
	}

	return best
}

func typeItems(scope analysis.Scope) []protocol.CompletionItem {
	names := scope.TypeNames()

	out := make([]protocol.CompletionItem, 0, len(names))
	kind := protocol.CompletionItemKindClass

	for _, name := range names {
		out = append(out, protocol.CompletionItem{Label: name, Kind: &kind})
	}

	return out
}

func relationItems(scope analysis.Scope, typeName string) []protocol.CompletionItem {
	relations := scope.RelationsOf(typeName)

	out := make([]protocol.CompletionItem, 0, len(relations))
	kind := protocol.CompletionItemKindProperty

	for _, rel := range relations {
		detail := rel.Expr
		item := protocol.CompletionItem{Label: rel.Name, Kind: &kind, Detail: &detail}

		if rel.Doc != "" {
			item.Documentation = protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: rel.Doc}
		}

		out = append(out, item)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })

	return out
}

func conditionItems(scope analysis.Scope) []protocol.CompletionItem {
	conds := scope.Conditions()

	out := make([]protocol.CompletionItem, 0, len(conds))
	kind := protocol.CompletionItemKindFunction

	for _, cond := range conds {
		detail := *conditionDetail(cond)
		out = append(out, protocol.CompletionItem{Label: cond.Name, Kind: &kind, Detail: &detail})
	}

	return out
}

func operatorItems() []protocol.CompletionItem {
	return []protocol.CompletionItem{
		keyword("or", "union", "or "),
		keyword("and", "intersection", "and "),
		keyword("but not", "exclusion", "but not "),
		keyword("from", "tupleset traversal", "from "),
	}
}

func declarationItems() []protocol.CompletionItem {
	return []protocol.CompletionItem{
		keyword("type", "type definition", "type ${1:name}\n  relations\n    define $0"),
		keyword("extend type", "extend a type from another module", "extend type ${1:name}\n  relations\n    define $0"),
		keyword("condition", "condition", "condition ${1:name}(${2:param}: ${3:string}) {\n  $0\n}"),
		keyword("module", "module header", "module ${1:name}"),
		keyword("model", "model header", "model\n  schema 1.1"),
	}
}

func keyword(label, detail, snippet string) protocol.CompletionItem {
	kind := protocol.CompletionItemKindKeyword
	format := protocol.InsertTextFormatSnippet

	return protocol.CompletionItem{
		Label:            label,
		Kind:             &kind,
		Detail:           &detail,
		InsertText:       &snippet,
		InsertTextFormat: &format,
	}
}

func trimWord(s string) string {
	i := len(s)
	for i > 0 && (isWordByte(s[i-1])) {
		i--
	}

	return s[:i]
}

func isWordByte(b byte) bool {
	return b == '_' || b == '-' || b == '.' || b == '/' ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// ---------------------------------------------------------------- yaml side

var (
	yamlRelationKey = regexp.MustCompile(`^\s*(-\s*)?relation:\s*[\w-]*$`)
	yamlTypeKey     = regexp.MustCompile(`^\s*(-\s*)?(user|object|type):\s*[\w./-]*$`)
	yamlAssertion   = regexp.MustCompile(`^\s+[\w-]*$`)
)

// storeCompletion offers the relations and types of the model a store test
// points at, in the three places a name can appear.
func storeCompletion(
	scope analysis.Scope,
	doc *analysis.Document,
	pos protocol.Position,
	prefix string,
) []protocol.CompletionItem {
	if doc.Store == nil {
		return nil
	}

	switch {
	case yamlRelationKey.MatchString(prefix):
		if objectType := tupleObjectType(doc.Store, pos.Line); objectType != "" {
			return relationItems(scope, objectType)
		}

		return allRelationItems(scope)

	case yamlTypeKey.MatchString(prefix):
		return typeItemsWithColon(scope)

	case yamlAssertion.MatchString(prefix):
		if objectType := checkObjectTypeAt(doc.Store, pos.Line); objectType != "" {
			return relationItems(scope, objectType)
		}
	}

	return nil
}

// tupleObjectType finds the object type of the tuple the cursor line is in.
func tupleObjectType(store *analysis.StoreTest, line protocol.UInteger) string {
	check := func(tuples []analysis.Tuple) string {
		for _, tuple := range tuples {
			if tuple.Range.Start.Line <= line && line <= tuple.Range.End.Line {
				objectType, _ := analysis.SplitObject(tuple.Object.Value)

				return objectType
			}
		}

		return ""
	}

	if found := check(store.Tuples); found != "" {
		return found
	}

	for _, test := range store.Tests {
		if found := check(test.Tuples); found != "" {
			return found
		}
	}

	return ""
}

func checkObjectTypeAt(store *analysis.StoreTest, line protocol.UInteger) string {
	for _, test := range store.Tests {
		for _, check := range test.Checks {
			if check.Range.Start.Line <= line && line <= check.Range.End.Line {
				return checkObjectType(check)
			}
		}

		for _, entry := range test.ListObjects {
			if entry.Range.Start.Line <= line && line <= entry.Range.End.Line {
				return entry.Type.Value
			}
		}

		for _, entry := range test.ListUsers {
			if entry.Range.Start.Line <= line && line <= entry.Range.End.Line {
				objectType, _ := analysis.SplitObject(entry.Object.Value)

				return objectType
			}
		}
	}

	return ""
}

func checkObjectType(check analysis.Check) string {
	for _, object := range append([]analysis.Field{check.Object}, check.Objects...) {
		if object.IsSet() {
			objectType, _ := analysis.SplitObject(object.Value)

			return objectType
		}
	}

	return ""
}

func typeItemsWithColon(scope analysis.Scope) []protocol.CompletionItem {
	names := scope.TypeNames()

	out := make([]protocol.CompletionItem, 0, len(names))
	kind := protocol.CompletionItemKindClass

	for _, name := range names {
		insert := name + ":"
		out = append(out, protocol.CompletionItem{Label: name, Kind: &kind, InsertText: &insert})
	}

	return out
}

func allRelationItems(scope analysis.Scope) []protocol.CompletionItem {
	seen := map[string]struct{}{}

	var out []protocol.CompletionItem

	kind := protocol.CompletionItemKindProperty

	for _, name := range scope.TypeNames() {
		for _, rel := range scope.RelationsOf(name) {
			if _, ok := seen[rel.Name]; ok {
				continue
			}

			seen[rel.Name] = struct{}{}

			detail := name

			out = append(out, protocol.CompletionItem{Label: rel.Name, Kind: &kind, Detail: &detail})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })

	return out
}
