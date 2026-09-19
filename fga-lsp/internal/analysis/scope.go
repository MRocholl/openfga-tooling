package analysis

import "sort"

// Scope is the set of model documents a name is resolved against.
type Scope struct {
	Docs []*Document
}

// ScopeFor returns the scope that bounds names used in doc.
func ScopeFor(v *View, doc *Document) Scope {
	switch doc.Kind {
	case KindModel:
		if _, members := v.ModuleSetFor(doc.Path); len(members) > 0 {
			return scopeOfPaths(v, members)
		}

		return Scope{Docs: v.ModelDocs()}

	case KindStoreTest:
		if doc.Store != nil && doc.Store.ModelPath != "" {
			if scope, ok := scopeOfModelRef(v, doc.Store.ModelPath); ok {
				return scope
			}
		}

		return Scope{Docs: v.ModelDocs()}

	case KindModFile:
		if doc.Mod != nil {
			var paths []string
			for _, entry := range doc.Mod.Contents {
				paths = append(paths, entry.Path)
			}

			if scope := scopeOfPaths(v, paths); len(scope.Docs) > 0 {
				return scope
			}
		}

		return Scope{Docs: v.ModelDocs()}

	case KindUnknown:
	}

	return Scope{Docs: v.ModelDocs()}
}

// scopeOfModelRef resolves a store's `model_file`, which is either an fga.mod or a single model.
func scopeOfModelRef(v *View, path string) (Scope, bool) {
	if mod := v.GetByPath(path); mod != nil && mod.Kind == KindModFile && mod.Mod != nil {
		var paths []string
		for _, entry := range mod.Mod.Contents {
			paths = append(paths, entry.Path)
		}

		if scope := scopeOfPaths(v, paths); len(scope.Docs) > 0 {
			return scope, true
		}
	}

	if model := v.GetByPath(path); model != nil && model.Kind == KindModel {
		return Scope{Docs: []*Document{model}}, true
	}

	return Scope{}, false
}

func scopeOfPaths(v *View, paths []string) Scope {
	var docs []*Document

	for _, path := range paths {
		if doc := v.GetByPath(path); doc != nil && doc.Kind == KindModel {
			docs = append(docs, doc)
		}
	}

	return Scope{Docs: docs}
}

func (s Scope) TypeDecls(name string) []*TypeDecl {
	var out []*TypeDecl

	for _, doc := range s.Docs {
		for _, decl := range doc.Types {
			if decl.Name == name {
				out = append(out, decl)
			}
		}
	}

	return out
}

func (s Scope) HasType(name string) bool { return len(s.TypeDecls(name)) > 0 }

func (s Scope) RelationDecls(typeName, relation string) []*RelationDecl {
	var out []*RelationDecl

	for _, decl := range s.TypeDecls(typeName) {
		for _, rel := range decl.Relations {
			if rel.Name == relation {
				out = append(out, rel)
			}
		}
	}

	return out
}

func (s Scope) HasRelation(typeName, relation string) bool {
	return len(s.RelationDecls(typeName, relation)) > 0
}

// RelationsOf lists a type's relations across every declaration of it, deduplicated by name and in...
func (s Scope) RelationsOf(typeName string) []*RelationDecl {
	seen := map[string]struct{}{}

	var out []*RelationDecl

	for _, decl := range s.TypeDecls(typeName) {
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

func (s Scope) TypeNames() []string {
	seen := map[string]struct{}{}

	var out []string

	for _, doc := range s.Docs {
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

func (s Scope) ConditionDecls(name string) []*ConditionDecl {
	var out []*ConditionDecl

	for _, doc := range s.Docs {
		for _, cond := range doc.Conds {
			if cond.Name == name {
				out = append(out, cond)
			}
		}
	}

	return out
}

func (s Scope) Conditions() []*ConditionDecl {
	var out []*ConditionDecl

	for _, doc := range s.Docs {
		out = append(out, doc.Conds...)
	}

	return out
}

// TuplesetTargets lists the types a tupleset relation can point at, which is where `X from tupleset` has to...
func (s Scope) TuplesetTargets(typeName, tupleset string) []string {
	seen := map[string]struct{}{}

	var out []string

	for _, decl := range s.RelationDecls(typeName, tupleset) {
		for _, ref := range decl.Refs {
			if ref.Kind != RefType {
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
