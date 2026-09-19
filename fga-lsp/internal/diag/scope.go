package diag

import (
	"sort"

	"github.com/mrocholl/fga-lsp/internal/analysis"
)

// scope is the set of model documents a name is resolved against.
type scope struct {
	docs      []*analysis.Document
	transient *analysis.Document
}

func (s scope) close() {
	if s.transient != nil {
		s.transient.Close()
	}
}

func scopeOf(view *analysis.View, docs []*analysis.Document) scope {
	if len(docs) == 0 {
		return scope{docs: view.ModelDocs()}
	}

	return scope{docs: docs}
}

func (s scope) typeDecls(name string) []*analysis.TypeDecl {
	var out []*analysis.TypeDecl

	for _, doc := range s.docs {
		for _, decl := range doc.Types {
			if decl.Name == name {
				out = append(out, decl)
			}
		}
	}

	return out
}

func (s scope) hasType(name string) bool { return len(s.typeDecls(name)) > 0 }

func (s scope) relationDecls(typeName, relation string) []*analysis.RelationDecl {
	var out []*analysis.RelationDecl

	for _, decl := range s.typeDecls(typeName) {
		for _, rel := range decl.Relations {
			if rel.Name == relation {
				out = append(out, rel)
			}
		}
	}

	return out
}

func (s scope) hasRelation(typeName, relation string) bool {
	return len(s.relationDecls(typeName, relation)) > 0
}

func (s scope) relationsOf(typeName string) []*analysis.RelationDecl {
	seen := map[string]struct{}{}

	var out []*analysis.RelationDecl

	for _, decl := range s.typeDecls(typeName) {
		for _, rel := range decl.Relations {
			if _, ok := seen[rel.Name]; ok {
				continue
			}

			seen[rel.Name] = struct{}{}

			out = append(out, rel)
		}
	}

	return out
}

func (s scope) typeNames() []string {
	seen := map[string]struct{}{}

	var out []string

	for _, doc := range s.docs {
		for _, decl := range doc.Types {
			if _, ok := seen[decl.Name]; ok {
				continue
			}

			seen[decl.Name] = struct{}{}

			out = append(out, decl.Name)
		}
	}

	sort.Strings(out)

	return out
}

func (s scope) conditionDecls(name string) []*analysis.ConditionDecl {
	var out []*analysis.ConditionDecl

	for _, doc := range s.docs {
		for _, cond := range doc.Conds {
			if cond.Name == name {
				out = append(out, cond)
			}
		}
	}

	return out
}

// tuplesetTargets lists the types a tupleset relation can point at.
func (s scope) tuplesetTargets(typeName, tupleset string) []string {
	seen := map[string]struct{}{}

	var out []string

	for _, decl := range s.relationDecls(typeName, tupleset) {
		for _, ref := range decl.Refs {
			if ref.Kind != analysis.RefType {
				continue
			}

			if _, ok := seen[ref.Name]; ok {
				continue
			}

			seen[ref.Name] = struct{}{}

			out = append(out, ref.Name)
		}
	}

	return out
}
