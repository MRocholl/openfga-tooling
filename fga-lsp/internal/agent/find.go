package agent

import (
	"sort"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/analysis"
	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/diag"
	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/feature"
)

// CheckWorkspace validates every model and store test, optionally narrowed to
// one file.
func (w *Workspace) CheckWorkspace(path string) CheckResult {
	result := CheckResult{}

	w.index.Read(func(v *analysis.View) {
		result.Models, result.Stores = counts(v)

		collected := diag.Result{}

		for _, doc := range v.Documents() {
			if doc.Kind == analysis.KindUnknown {
				continue
			}

			if path != "" && rel(w.root, doc.Path) != path && doc.Path != path {
				continue
			}

			for uri, diags := range diag.Compute(v, doc) {
				if len(diags) > 0 {
					collected[uri] = append(collected[uri], diags...)
				}
			}
		}

		for uri, diags := range collected {
			doc := v.Get(uri)

			for _, d := range dedupe(diags) {
				result.Problems = append(result.Problems, Problem{
					Location: w.locate(doc, uri, d.Range),
					Severity: severity(d),
					Message:  d.Message,
					Excerpt:  excerpt(doc, d.Range, true),
				})
			}
		}
	})

	sortLocations(result.Problems, func(p Problem) Location { return p.Location })

	return result
}

// FindReferences collects every mention of a name.
//
// Nothing is dropped by default: an agent deciding whether a relation is safe
// to remove needs the whole answer, and a silently shortened list is worse
// than a long one. Truncation is opt-in through limit, and what it left out
// is reported.
func (w *Workspace) FindReferences(name string, limit int) ReferenceResult {
	typeName, relation := splitName(name)
	result := ReferenceResult{Name: name}

	w.index.Read(func(v *analysis.View) {
		scope := w.scope(v)

		if relation == "" && !scope.HasType(typeName) {
			if owners := w.owners(scope, typeName); len(owners) > 0 {
				result.Candidates = owners
				result.Note = "`" + typeName + "` is a relation; ask again with one of these names"

				return
			}
		}

		want := feature.Target{Kind: feature.TargetType, Name: typeName}
		if relation != "" {
			want = feature.Target{
				Kind:       feature.TargetRelation,
				Name:       relation,
				OwnerTypes: []string{typeName},
			}
		}

		counts := map[string]int{}

		for _, doc := range v.Documents() {
			for _, symbol := range feature.Symbols(v, doc) {
				if symbol.Kind != want.Kind || symbol.Name != want.Name {
					continue
				}

				if want.Kind == feature.TargetRelation && len(symbol.OwnerTypes) > 0 &&
					!contains(symbol.OwnerTypes, typeName) {
					continue
				}

				ref := Reference{
					Location: w.locate(doc, doc.URI, symbol.Range),
					Kind:     refKind(doc, symbol),
				}

				result.References = append(result.References, ref)
				counts[ref.Path]++
			}
		}

		if len(result.References) == 0 {
			result.Candidates = nearest(typeName, scope.TypeNames(), 5)

			return
		}

		for path, count := range counts {
			result.ByFile = append(result.ByFile, FileCount{Path: path, Count: count})
		}

		sort.Slice(result.ByFile, func(i, j int) bool {
			if result.ByFile[i].Count != result.ByFile[j].Count {
				return result.ByFile[i].Count > result.ByFile[j].Count
			}

			return result.ByFile[i].Path < result.ByFile[j].Path
		})
	})

	sortLocations(result.References, func(r Reference) Location { return r.Location })

	result.Total = len(result.References)

	if limit > 0 && result.Total > limit {
		result.Omitted = result.Total - limit
		result.References = result.References[:limit]
	}

	return result
}

func refKind(doc *analysis.Document, symbol feature.Target) string {
	switch {
	case doc.Kind == analysis.KindStoreTest:
		return RefKindTest
	case symbol.Declaration:
		return RefKindDefinition
	default:
		return RefKindUse
	}
}

// FindDefinition locates where a name is declared.
func (w *Workspace) FindDefinition(name string) DefinitionResult {
	typeName, relation := splitName(name)
	result := DefinitionResult{Name: name}

	w.index.Read(func(v *analysis.View) {
		scope := w.scope(v)

		if relation == "" {
			for _, decl := range scope.TypeDecls(typeName) {
				doc := v.Get(decl.URI)
				result.Definitions = append(result.Definitions, Definition{
					Location: w.locate(doc, decl.URI, decl.NameRange),
					Name:     decl.Name,
					Doc:      decl.Doc,
					Excerpt:  excerpt(doc, decl.Range, false),
				})
			}

			if len(result.Definitions) == 0 {
				if owners := w.owners(scope, typeName); len(owners) > 0 {
					result.Candidates = owners
					result.Note = "`" + typeName + "` is a relation; ask again with one of these names"

					return
				}

				result.Candidates = nearest(typeName, scope.TypeNames(), 5)
			}

			return
		}

		for _, decl := range scope.RelationDecls(typeName, relation) {
			doc := v.Get(decl.URI)
			result.Definitions = append(result.Definitions, Definition{
				Location: w.locate(doc, decl.URI, decl.NameRange),
				Name:     qualify(decl.TypeName, decl.Name),
				Doc:      decl.Doc,
				Excerpt:  excerpt(doc, decl.Range, false),
			})
		}

		if len(result.Definitions) == 0 {
			var candidates []string
			for _, rel := range scope.RelationsOf(typeName) {
				candidates = append(candidates, rel.Name)
			}

			for _, near := range nearest(relation, candidates, 5) {
				result.Candidates = append(result.Candidates, qualify(typeName, near))
			}
		}
	})

	return result
}

// FindSymbols matches names across the workspace.
func (w *Workspace) FindSymbols(query string, limit int) SearchResult {
	result := SearchResult{Query: query}

	w.index.Read(func(v *analysis.View) {
		for _, symbol := range feature.WorkspaceSymbols(v, query) {
			uri := protocol.DocumentUri(symbol.Location.URI)

			container := ""
			if symbol.ContainerName != nil {
				container = *symbol.ContainerName
			}

			result.Symbols = append(result.Symbols, Symbol{
				Location:  w.locate(v.Get(uri), uri, symbol.Location.Range),
				Name:      symbol.Name,
				Kind:      symbolKind(symbol.Kind),
				Container: container,
			})
		}
	})

	if limit > 0 && len(result.Symbols) > limit {
		result.Omitted = len(result.Symbols) - limit
		result.Symbols = result.Symbols[:limit]
	}

	return result
}

func symbolKind(kind protocol.SymbolKind) string {
	switch kind {
	case protocol.SymbolKindClass:
		return "type"
	case protocol.SymbolKindProperty:
		return "relation"
	case protocol.SymbolKindFunction:
		return "condition"
	default:
		return "symbol"
	}
}

// owners lists the types defining a relation of this name.
func (w *Workspace) owners(scope analysis.Scope, relation string) []string {
	var out []string

	for _, typeName := range scope.TypeNames() {
		if scope.HasRelation(typeName, relation) {
			out = append(out, qualify(typeName, relation))
		}
	}

	return out
}

func (w *Workspace) locate(doc *analysis.Document, uri protocol.DocumentUri, rng protocol.Range) Location {
	loc := Location{
		Path:      rel(w.root, analysis.PathFromURI(uri)),
		Line:      int(rng.Start.Line) + 1,
		Column:    int(rng.Start.Character) + 1,
		EndColumn: int(rng.End.Character) + 1,
	}

	if doc != nil {
		loc.Text = strings.TrimSpace(string(doc.Lines.LineBytes(int(rng.Start.Line))))
	}

	return loc
}

func sortLocations[T any](items []T, key func(T) Location) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := key(items[i]), key(items[j])

		if a.Path != b.Path {
			return a.Path < b.Path
		}

		if a.Line != b.Line {
			return a.Line < b.Line
		}

		return a.Column < b.Column
	})
}
