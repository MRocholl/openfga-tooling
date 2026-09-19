package feature

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/mrocholl/fga-lsp/internal/analysis"
)

// DocumentSymbols outlines a model file: types, each with its relations
// nested underneath, and any conditions.
func DocumentSymbols(doc *analysis.Document) []protocol.DocumentSymbol {
	if doc.Kind != analysis.KindModel {
		return nil
	}

	out := make([]protocol.DocumentSymbol, 0, len(doc.Types)+len(doc.Conds))

	for _, decl := range doc.Types {
		symbol := protocol.DocumentSymbol{
			Name:           decl.Name,
			Detail:         detailFor(decl),
			Kind:           protocol.SymbolKindClass,
			Range:          decl.Range,
			SelectionRange: decl.NameRange,
		}

		for _, rel := range decl.Relations {
			expr := rel.Expr

			symbol.Children = append(symbol.Children, protocol.DocumentSymbol{
				Name:           rel.Name,
				Detail:         &expr,
				Kind:           protocol.SymbolKindProperty,
				Range:          rel.Range,
				SelectionRange: rel.NameRange,
			})
		}

		out = append(out, symbol)
	}

	for _, cond := range doc.Conds {
		out = append(out, protocol.DocumentSymbol{
			Name:           cond.Name,
			Detail:         conditionDetail(cond),
			Kind:           protocol.SymbolKindFunction,
			Range:          cond.Range,
			SelectionRange: cond.NameRange,
		})
	}

	return out
}

// WorkspaceSymbols answers a fuzzy query across every model in the workspace.
func WorkspaceSymbols(v *analysis.View, query string) []protocol.SymbolInformation {
	var out []protocol.SymbolInformation

	for _, doc := range v.ModelDocs() {
		for _, decl := range doc.Types {
			if fuzzyMatch(query, decl.Name) {
				out = append(out, symbolInfo(decl.Name, decl.Module, protocol.SymbolKindClass, doc.URI, decl.NameRange))
			}

			for _, rel := range decl.Relations {
				if fuzzyMatch(query, rel.Name) {
					out = append(out, symbolInfo(
						rel.Name, decl.Name, protocol.SymbolKindProperty, doc.URI, rel.NameRange,
					))
				}
			}
		}

		for _, cond := range doc.Conds {
			if fuzzyMatch(query, cond.Name) {
				out = append(out, symbolInfo(
					cond.Name, cond.Module, protocol.SymbolKindFunction, doc.URI, cond.NameRange,
				))
			}
		}
	}

	return out
}

func symbolInfo(
	name, container string,
	kind protocol.SymbolKind,
	uri protocol.DocumentUri,
	rng protocol.Range,
) protocol.SymbolInformation {
	info := protocol.SymbolInformation{
		Name:     name,
		Kind:     kind,
		Location: protocol.Location{URI: string(uri), Range: rng},
	}

	if container != "" {
		info.ContainerName = &container
	}

	return info
}

// fuzzyMatch accepts a name whose characters contain the query's in order,
// which is what an editor's symbol prompt expects of `cvw` -> `can_view`.
func fuzzyMatch(query, name string) bool {
	if query == "" {
		return true
	}

	query, name = strings.ToLower(query), strings.ToLower(name)

	i := 0
	for j := 0; j < len(name) && i < len(query); j++ {
		if name[j] == query[i] {
			i++
		}
	}

	return i == len(query)
}

func detailFor(decl *analysis.TypeDecl) *string {
	detail := decl.Module
	if decl.Extend {
		detail = strings.TrimSpace(detail + " (extend)")
	}

	if detail == "" {
		return nil
	}

	return &detail
}

func conditionDetail(cond *analysis.ConditionDecl) *string {
	params := make([]string, 0, len(cond.Params))
	for _, param := range cond.Params {
		params = append(params, param.Name+": "+param.Type)
	}

	detail := "(" + strings.Join(params, ", ") + ")"

	return &detail
}
