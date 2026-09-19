package diag

import (
	"fmt"
	"regexp"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/mrocholl/fga-lsp/internal/analysis"
)

// The semantic checks below are deliberately worded and positioned to match
// openfga/language's own validator, case for case, against the corpus in
// testdata/upstream. Reusing its wording is not pedantry: it is what makes a
// squiggle here say the same thing as `fga model validate` in the terminal
// and as the official editor plugins, so nobody has to learn two vocabularies
// for one mistake.
//
// The type and relation naming rules are openfga's, reproduced here because
// the reference implementation quotes them verbatim in its message.
var (
	typeNamePattern     = regexp.MustCompile(`^[^:#@*\s]{1,254}$`)
	relationNamePattern = regexp.MustCompile(`^[^:#@*\s]{1,50}$`)
)

const (
	typeNameRule     = `^[^:#@\*\s]{1,254}$`
	relationNameRule = `^[^:#@\*\s]{1,50}$`
)

// reservedNames may not be used for a type or a relation: both already mean
// something inside a relation definition.
var reservedNames = map[string]bool{"self": true, "this": true}

// supportedSchemas are the schema versions this server, and the `fga` release
// it is pinned to, understand.
var supportedSchemas = map[string]bool{"1.1": true, "1.2": true}

// semanticErrors reports everything that is wrong with a model that is not a
// syntax error, in upstream's terms. covered records what it reported, so the
// typesystem pass can avoid saying the same thing again in its own words.
func semanticErrors(
	names scope,
	doc *analysis.Document,
	inModuleSet bool,
	result Result,
	covered map[string]struct{},
) {
	schemaErrors(doc, inModuleSet, result)
	duplicateTypes(names, doc, result)

	for _, typeDecl := range doc.Types {
		nameErrors(typeDecl, result, doc)

		seen := map[string]bool{}

		for _, rel := range typeDecl.Relations {
			if seen[rel.Name] {
				result.add(doc.URI, diagnostic(
					rel.NameRange,
					fmt.Sprintf("the relation `%s` is a duplicate.", rel.Name),
					protocol.DiagnosticSeverityError,
				))
			}

			seen[rel.Name] = true

			duplicatePartials(rel, doc, result)
			duplicateRestrictions(rel, doc, result)

			for _, ref := range rel.Refs {
				problems := resolveRef(names, typeDecl.Name, ref)
				if len(problems) == 0 {
					continue
				}

				for _, p := range problems {
					result.add(doc.URI, diagnostic(p.rng, p.message, protocol.DiagnosticSeverityError))
				}

				for _, key := range refKeys(names, typeDecl.Name, ref) {
					covered[key] = struct{}{}
				}
			}
		}
	}

	unusedConditions(names, doc, result)
}

// schemaErrors checks the `model` header. A module inside an fga.mod takes
// its schema version from there and carries none of its own.
func schemaErrors(doc *analysis.Document, inModuleSet bool, result Result) {
	if inModuleSet {
		return
	}

	if !doc.HasHeader {
		result.add(doc.URI, diagnostic(
			protocol.Range{},
			"schema version required",
			protocol.DiagnosticSeverityError,
		))

		return
	}

	if doc.Schema != "" && !supportedSchemas[doc.Schema] {
		result.add(doc.URI, diagnostic(
			doc.SchemaRange,
			"invalid schema "+doc.Schema,
			protocol.DiagnosticSeverityError,
		))
	}
}

func nameErrors(typeDecl *analysis.TypeDecl, result Result, doc *analysis.Document) {
	switch {
	case reservedNames[typeDecl.Name]:
		result.add(doc.URI, diagnostic(
			typeDecl.NameRange,
			"a type cannot be named 'self' or 'this'.",
			protocol.DiagnosticSeverityError,
		))
	case !typeNamePattern.MatchString(typeDecl.Name):
		result.add(doc.URI, diagnostic(
			typeDecl.NameRange,
			fmt.Sprintf("type '%s' does not match naming rule: '%s'.", typeDecl.Name, typeNameRule),
			protocol.DiagnosticSeverityError,
		))
	}

	for _, rel := range typeDecl.Relations {
		switch {
		case reservedNames[rel.Name]:
			result.add(doc.URI, diagnostic(
				rel.NameRange,
				"a relation cannot be named 'self' or 'this'.",
				protocol.DiagnosticSeverityError,
			))
		case !relationNamePattern.MatchString(rel.Name):
			result.add(doc.URI, diagnostic(
				rel.NameRange,
				fmt.Sprintf(
					"relation '%s' of type '%s' does not match naming rule: '%s'.",
					rel.Name, typeDecl.Name, relationNameRule,
				),
				protocol.DiagnosticSeverityError,
			))
		}
	}
}

// duplicateTypes reports a type declared twice. An `extend type` is not a
// redeclaration, which is the whole point of it.
func duplicateTypes(names scope, doc *analysis.Document, result Result) {
	for _, typeDecl := range doc.Types {
		if typeDecl.Extend {
			continue
		}

		first := true

		for _, other := range names.typeDecls(typeDecl.Name) {
			if other.Extend {
				continue
			}

			if other == typeDecl {
				break
			}

			first = false

			break
		}

		if first && countDeclarations(names, typeDecl.Name) > 1 {
			result.add(doc.URI, diagnostic(
				typeDecl.NameRange,
				fmt.Sprintf("the type `%s` is a duplicate.", typeDecl.Name),
				protocol.DiagnosticSeverityError,
			))
		}
	}
}

func countDeclarations(names scope, typeName string) int {
	count := 0

	for _, decl := range names.typeDecls(typeName) {
		if !decl.Extend {
			count++
		}
	}

	return count
}

// duplicatePartials reports `define viewer: a or a`.
func duplicatePartials(rel *analysis.RelationDecl, doc *analysis.Document, result Result) {
	if len(rel.Partials) < 2 {
		return
	}

	seen := map[string]bool{}

	for _, partial := range rel.Partials {
		if seen[partial.Text] {
			result.add(doc.URI, diagnostic(
				firstOccurrence(rel.Partials, partial.Text),
				fmt.Sprintf(
					"the partial relation definition `%s` is a duplicate in the relation `%s`.",
					partial.Text, rel.Name,
				),
				protocol.DiagnosticSeverityError,
			))
		}

		seen[partial.Text] = true
	}
}

// duplicateRestrictions reports `define viewer: [user, user]`.
func duplicateRestrictions(rel *analysis.RelationDecl, doc *analysis.Document, result Result) {
	seen := map[string]bool{}

	for _, ref := range rel.Refs {
		if ref.Kind != analysis.RefType {
			continue
		}

		if seen[ref.Text] {
			result.add(doc.URI, diagnostic(
				firstRestriction(rel.Refs, ref.Text),
				fmt.Sprintf(
					"the type restriction `%s` is a duplicate in the relation `%s`.",
					ref.Text, rel.Name,
				),
				protocol.DiagnosticSeverityError,
			))
		}

		seen[ref.Text] = true
	}
}

func firstOccurrence(partials []analysis.Partial, text string) protocol.Range {
	for _, partial := range partials {
		if partial.Text == text {
			return partial.Range
		}
	}

	return protocol.Range{}
}

func firstRestriction(refs []analysis.Ref, text string) protocol.Range {
	for _, ref := range refs {
		if ref.Kind == analysis.RefType && ref.Text == text {
			return ref.OuterRange
		}
	}

	return protocol.Range{}
}

// resolveRef reports what is wrong with one reference, or nothing when it
// resolves. A tupleset whose relation points at several types yields one
// problem per type, which is how upstream reports it.
func resolveRef(names scope, enclosingType string, ref analysis.Ref) []problem {
	one := func(message string, rng protocol.Range) []problem {
		return []problem{{message: message, rng: rng}}
	}

	switch ref.Kind {
	case analysis.RefType:
		if !names.hasType(ref.Name) {
			return one(fmt.Sprintf("`%s` is not a valid type.", ref.Name), ref.Range)
		}

	case analysis.RefRelationOnSelf:
		if !names.hasRelation(enclosingType, ref.Name) {
			return one(fmt.Sprintf("the relation `%s` does not exist.", ref.Name), ref.Range)
		}

	case analysis.RefRelationOnType:
		if !names.hasType(ref.OwnerType) {
			return nil // the type itself is already reported
		}

		if !names.hasRelation(ref.OwnerType, ref.Name) {
			return one(
				fmt.Sprintf("`%s` is not a valid relation for `%s`.", ref.Name, ref.OwnerType),
				ref.OuterRange,
			)
		}

	case analysis.RefTupleset:
		// The name to the right of `from` has to be a relation of the
		// enclosing type, and one that names plain types.
		decls := names.relationDecls(enclosingType, ref.Name)
		if len(decls) == 0 {
			return one(
				fmt.Sprintf("`%s` is not a valid relation for `%s`.", ref.Name, enclosingType),
				ref.OuterRange,
			)
		}

		for _, decl := range decls {
			if !decl.DirectOnly {
				return one(
					fmt.Sprintf("`%s` relation used inside from allows only direct relation.", ref.Name),
					ref.Range,
				)
			}
		}

	case analysis.RefRelationViaTupleset:
		targets := names.tuplesetTargets(enclosingType, ref.Tupleset)
		if len(targets) == 0 {
			return nil // the tupleset itself is already reported
		}

		var problems []problem

		for _, target := range targets {
			if names.hasRelation(target, ref.Name) {
				return nil
			}

			problems = append(problems, problem{
				message: fmt.Sprintf(
					"the `%s` relation definition on type `%s` is not valid: "+
						"`%s` does not exist on `%s`, which is of type `%s`.",
					ref.Name, enclosingType, ref.Name, ref.Tupleset, target,
				),
				rng: ref.OuterRange,
			})
		}

		return problems

	case analysis.RefCondition:
		if len(names.conditionDecls(ref.Name)) == 0 {
			return one(
				fmt.Sprintf("`%s` is not a defined condition in the model.", ref.Name),
				ref.Range,
			)
		}
	}

	return nil
}

// unusedConditions reports a condition no type restriction mentions. A
// condition that nothing applies is dead weight the model carries into every
// evaluation.
func unusedConditions(names scope, doc *analysis.Document, result Result) {
	used := map[string]bool{}

	for _, model := range names.docs {
		for _, typeDecl := range model.Types {
			for _, rel := range typeDecl.Relations {
				for _, ref := range rel.Refs {
					if ref.Kind == analysis.RefCondition {
						used[ref.Name] = true
					}
				}
			}
		}
	}

	for _, cond := range doc.Conds {
		if used[cond.Name] {
			continue
		}

		result.add(doc.URI, diagnostic(
			cond.NameRange,
			fmt.Sprintf("`%s` condition is not used in the model.", cond.Name),
			protocol.DiagnosticSeverityError,
		))
	}
}

// refKeys names what a failed reference was looking for, in the same terms
// the typesystem reports its own failures in.
func refKeys(names scope, enclosingType string, ref analysis.Ref) []string {
	switch ref.Kind {
	case analysis.RefType:
		return []string{subjectKey(ref.Name, "")}

	case analysis.RefRelationOnSelf, analysis.RefTupleset:
		return []string{subjectKey(enclosingType, ref.Name)}

	case analysis.RefRelationOnType:
		return []string{subjectKey(ref.OwnerType, ref.Name)}

	case analysis.RefRelationViaTupleset:
		targets := names.tuplesetTargets(enclosingType, ref.Tupleset)

		keys := make([]string, 0, len(targets))
		for _, target := range targets {
			keys = append(keys, subjectKey(target, ref.Name))
		}

		return keys

	case analysis.RefCondition:
		return []string{"condition:" + ref.Name}
	}

	return nil
}

func subjectKey(objectType, relation string) string {
	if relation == "" {
		return objectType
	}

	return objectType + "#" + relation
}
