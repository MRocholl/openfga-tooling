// Package feature implements the language features the server exposes.
//
// Everything routes through Target: a position in any of the three file kinds
// resolves to the same small vocabulary -- a type, a relation on some type, a
// condition, a module, or a file reference -- and each feature then works in
// those terms rather than in tree nodes and YAML scalars.
package feature

import (
	"strings"
	"unicode/utf16"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/mrocholl/fga-lsp/internal/analysis"
)

type TargetKind int

const (
	TargetNone TargetKind = iota
	TargetType
	TargetRelation
	TargetCondition
	TargetModule
	// TargetFileRef is a path written in an fga.mod or a store's model_file.
	TargetFileRef
)

// Target is whatever the cursor is on, named in model terms.
type Target struct {
	Kind TargetKind
	Name string
	// OwnerTypes are the types a relation may belong to. A relation reached
	// through a tupleset can have several.
	OwnerTypes []string
	Range      protocol.Range
	// Path is set for TargetFileRef.
	Path string
	// Declaration is true when the cursor sits on the defining occurrence
	// rather than a use.
	Declaration bool
}

func (t Target) Found() bool { return t.Kind != TargetNone }

// TargetAt resolves the position in doc.
func TargetAt(v *analysis.View, doc *analysis.Document, pos protocol.Position) Target {
	for _, target := range Symbols(v, doc) {
		if analysis.InRange(target.Range, pos) {
			return target
		}
	}

	return Target{}
}

// Symbols enumerates every resolvable name in doc, declarations and uses
// alike. Position lookup, find-references and rename all read from this one
// list, so they cannot disagree about what a given span means.
func Symbols(v *analysis.View, doc *analysis.Document) []Target {
	switch doc.Kind {
	case analysis.KindModel:
		return modelSymbols(analysis.ScopeFor(v, doc), doc)
	case analysis.KindStoreTest:
		return storeSymbols(doc)
	case analysis.KindModFile:
		return modSymbols(doc)
	case analysis.KindUnknown:
	}

	return nil
}

func modelSymbols(scope analysis.Scope, doc *analysis.Document) []Target {
	var out []Target

	for _, decl := range doc.Types {
		out = append(out, Target{
			Kind:        TargetType,
			Name:        decl.Name,
			Range:       decl.NameRange,
			Declaration: true,
		})

		for _, rel := range decl.Relations {
			out = append(out, Target{
				Kind:        TargetRelation,
				Name:        rel.Name,
				OwnerTypes:  []string{decl.Name},
				Range:       rel.NameRange,
				Declaration: true,
			})

			for _, ref := range rel.Refs {
				if target := targetFromRef(scope, decl.Name, ref); target.Found() {
					out = append(out, target)
				}
			}
		}
	}

	for _, cond := range doc.Conds {
		out = append(out, Target{
			Kind:        TargetCondition,
			Name:        cond.Name,
			Range:       cond.NameRange,
			Declaration: true,
		})
	}

	return out
}

func targetFromRef(scope analysis.Scope, enclosingType string, ref analysis.Ref) Target {
	switch ref.Kind {
	case analysis.RefType:
		return Target{Kind: TargetType, Name: ref.Name, Range: ref.Range}

	case analysis.RefRelationOnSelf:
		return Target{
			Kind:       TargetRelation,
			Name:       ref.Name,
			OwnerTypes: []string{enclosingType},
			Range:      ref.Range,
		}

	case analysis.RefRelationOnType:
		return Target{
			Kind:       TargetRelation,
			Name:       ref.Name,
			OwnerTypes: []string{ref.OwnerType},
			Range:      ref.Range,
		}

	case analysis.RefRelationViaTupleset:
		return Target{
			Kind:       TargetRelation,
			Name:       ref.Name,
			OwnerTypes: scope.TuplesetTargets(enclosingType, ref.Tupleset),
			Range:      ref.Range,
		}

	case analysis.RefCondition:
		return Target{Kind: TargetCondition, Name: ref.Name, Range: ref.Range}
	}

	return Target{}
}

// --------------------------------------------------------------- store tests

func storeSymbols(doc *analysis.Document) []Target {
	store := doc.Store
	if store == nil {
		return nil
	}

	var out []Target

	if store.ModelFile.IsSet() {
		out = append(out, Target{
			Kind:  TargetFileRef,
			Name:  store.ModelFile.Value,
			Path:  store.ModelPath,
			Range: store.ModelFile.Range,
		})
	}

	out = appendTuples(out, store.Tuples)

	for _, test := range store.Tests {
		out = appendTuples(out, test.Tuples)

		for _, check := range test.Checks {
			objectType := ""

			for _, object := range append([]analysis.Field{check.Object}, check.Objects...) {
				if target := objectSymbol(object); target.Found() {
					out = append(out, target)

					if objectType == "" {
						objectType = target.Name
					}
				}
			}

			for _, user := range append([]analysis.Field{check.User}, check.Users...) {
				out = append(out, userSymbols(user)...)
			}

			for _, assertion := range check.Assertions {
				out = append(out, Target{
					Kind:       TargetRelation,
					Name:       assertion.Relation.Value,
					OwnerTypes: nonEmpty(objectType),
					Range:      assertion.Relation.Range,
				})
			}
		}

		for _, entry := range test.ListObjects {
			out = append(out, userSymbols(entry.User)...)

			if entry.Type.IsSet() {
				out = append(out, Target{Kind: TargetType, Name: entry.Type.Value, Range: entry.Type.Range})
			}

			for _, assertion := range entry.Assertions {
				out = append(out, Target{
					Kind:       TargetRelation,
					Name:       assertion.Value,
					OwnerTypes: nonEmpty(entry.Type.Value),
					Range:      assertion.Range,
				})
			}
		}

		for _, entry := range test.ListUsers {
			objectType := ""

			if target := objectSymbol(entry.Object); target.Found() {
				out = append(out, target)
				objectType = target.Name
			}

			for _, filter := range entry.UserFilter {
				if filter.IsSet() {
					out = append(out, Target{Kind: TargetType, Name: filter.Value, Range: filter.Range})
				}
			}

			for _, assertion := range entry.Assertions {
				out = append(out, Target{
					Kind:       TargetRelation,
					Name:       assertion.Value,
					OwnerTypes: nonEmpty(objectType),
					Range:      assertion.Range,
				})
			}
		}
	}

	return out
}

func appendTuples(out []Target, tuples []analysis.Tuple) []Target {
	for _, tuple := range tuples {
		objectType := ""

		if target := objectSymbol(tuple.Object); target.Found() {
			out = append(out, target)
			objectType = target.Name
		}

		out = append(out, userSymbols(tuple.User)...)

		if tuple.Relation.IsSet() {
			out = append(out, Target{
				Kind:       TargetRelation,
				Name:       tuple.Relation.Value,
				OwnerTypes: nonEmpty(objectType),
				Range:      tuple.Relation.Range,
			})
		}
	}

	return out
}

// objectSymbol reads `document:roadmap`, narrowing the range to the type half
// so a rename does not touch the id.
func objectSymbol(field analysis.Field) Target {
	if !field.IsSet() {
		return Target{}
	}

	objectType, _ := analysis.SplitObject(field.Value)
	if objectType == "" {
		return Target{}
	}

	return Target{
		Kind:  TargetType,
		Name:  objectType,
		Range: subRange(field, 0, len(objectType)),
	}
}

// userSymbols reads `user:alice`, `user:*` and `team:eng#member`, yielding the
// type and, where present, the userset relation.
func userSymbols(field analysis.Field) []Target {
	if !field.IsSet() {
		return nil
	}

	userType, _, relation := analysis.SplitTupleUser(field.Value)
	if userType == "" {
		return nil
	}

	out := []Target{{
		Kind:  TargetType,
		Name:  userType,
		Range: subRange(field, 0, len(userType)),
	}}

	if relation != "" {
		hash := strings.Index(field.Value, "#")

		out = append(out, Target{
			Kind:       TargetRelation,
			Name:       relation,
			OwnerTypes: nonEmpty(userType),
			Range:      subRange(field, hash+1, len(field.Value)),
		})
	}

	return out
}

// ------------------------------------------------------------------ fga.mod

func modSymbols(doc *analysis.Document) []Target {
	if doc.Mod == nil {
		return nil
	}

	out := make([]Target, 0, len(doc.Mod.Contents))
	for _, entry := range doc.Mod.Contents {
		out = append(out, Target{Kind: TargetFileRef, Name: entry.Value, Path: entry.Path, Range: entry.Range})
	}

	return out
}

// ------------------------------------------------------------------ helpers

// subRange narrows a single-line field range to the byte span [start, end) of
// its value, converting to the UTF-16 offsets LSP counts in.
func subRange(field analysis.Field, start, end int) protocol.Range {
	if start < 0 || end > len(field.Value) || start > end {
		return field.Range
	}

	base := field.Range.Start

	return protocol.Range{
		Start: protocol.Position{
			Line:      base.Line,
			Character: base.Character + protocol.UInteger(utf16Width(field.Value[:start])),
		},
		End: protocol.Position{
			Line:      base.Line,
			Character: base.Character + protocol.UInteger(utf16Width(field.Value[:end])),
		},
	}
}

func utf16Width(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}

	return n
}

func nonEmpty(values ...string) []string {
	var out []string

	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}

	return out
}
