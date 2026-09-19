package feature

import (
	"slices"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/mrocholl/fga-lsp/internal/analysis"
)

// Definition resolves the declarations a target points at. A relation reached
// through a tupleset, or a type extended in several modules, legitimately has
// more than one.
func Definition(v *analysis.View, doc *analysis.Document, pos protocol.Position) []protocol.Location {
	target := TargetAt(v, doc, pos)
	if !target.Found() {
		return nil
	}

	scope := analysis.ScopeFor(v, doc)

	return declarations(scope, target)
}

func declarations(scope analysis.Scope, target Target) []protocol.Location {
	var out []protocol.Location

	switch target.Kind {
	case TargetType:
		for _, decl := range scope.TypeDecls(target.Name) {
			out = append(out, protocol.Location{URI: string(decl.URI), Range: decl.NameRange})
		}

	case TargetRelation:
		for _, owner := range target.OwnerTypes {
			for _, decl := range scope.RelationDecls(owner, target.Name) {
				out = append(out, protocol.Location{URI: string(decl.URI), Range: decl.NameRange})
			}
		}

	case TargetCondition:
		for _, decl := range scope.ConditionDecls(target.Name) {
			out = append(out, protocol.Location{URI: string(decl.URI), Range: decl.NameRange})
		}

	case TargetFileRef:
		if target.Path != "" {
			out = append(out, protocol.Location{URI: string(analysis.URIFromPath(target.Path))})
		}

	case TargetModule, TargetNone:
	}

	return out
}

// References finds every mention of the target across the workspace: the
// model files in its scope, and any store test that resolves to that model.
func References(
	v *analysis.View,
	doc *analysis.Document,
	pos protocol.Position,
	includeDeclaration bool,
) []protocol.Location {
	target := TargetAt(v, doc, pos)
	if !target.Found() {
		return nil
	}

	var out []protocol.Location

	for _, candidate := range relatedDocuments(v, doc) {
		for _, symbol := range Symbols(v, candidate) {
			if !matches(symbol, target) {
				continue
			}

			if symbol.Declaration && !includeDeclaration {
				continue
			}

			out = append(out, protocol.Location{URI: string(candidate.URI), Range: symbol.Range})
		}
	}

	return out
}

// matches reports whether a symbol denotes the same thing as the target.
//
// Relations are compared by name and owning type. A symbol whose owner is
// unknown -- a tuple whose object type did not resolve, say -- matches on
// name alone rather than being dropped, because the alternative is a rename
// that silently misses occurrences.
func matches(symbol, target Target) bool {
	if symbol.Kind != target.Kind || symbol.Name != target.Name {
		return false
	}

	if symbol.Kind != TargetRelation {
		return true
	}

	if len(symbol.OwnerTypes) == 0 || len(target.OwnerTypes) == 0 {
		return true
	}

	for _, owner := range symbol.OwnerTypes {
		if slices.Contains(target.OwnerTypes, owner) {
			return true
		}
	}

	return false
}

// relatedDocuments returns the documents a reference search has to look at:
// the model files in scope, plus the store tests and fga.mod files that name
// them.
func relatedDocuments(v *analysis.View, doc *analysis.Document) []*analysis.Document {
	scope := analysis.ScopeFor(v, doc)

	inScope := make(map[protocol.DocumentUri]struct{}, len(scope.Docs))

	out := make([]*analysis.Document, 0, len(scope.Docs))

	for _, model := range scope.Docs {
		inScope[model.URI] = struct{}{}

		out = append(out, model)
	}

	for _, candidate := range v.Documents() {
		if candidate.Kind != analysis.KindStoreTest || candidate.Store == nil {
			continue
		}

		for _, model := range analysis.ScopeFor(v, candidate).Docs {
			if _, ok := inScope[model.URI]; ok {
				out = append(out, candidate)

				break
			}
		}
	}

	if _, ok := inScope[doc.URI]; !ok && doc.Kind != analysis.KindStoreTest {
		out = append(out, doc)
	}

	return out
}
