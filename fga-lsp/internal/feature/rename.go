package feature

import (
	"errors"
	"fmt"
	"regexp"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/mrocholl/fga-lsp/internal/analysis"
)

// identifierPattern is the reference lexer's EXTENDED_IDENTIFIER, which is
// what a type or relation name has to be.
var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*([/.-]?[A-Za-z0-9_]+)*$`)

var (
	ErrNotRenameable = errors.New("only types, relations and conditions can be renamed")
	ErrNotDeclared   = errors.New("cannot rename a name that is not declared in this model")
)

// PrepareRename tells the editor which span it is about to rename, and
// refuses early for anything that has no declaration to move.
func PrepareRename(
	v *analysis.View,
	doc *analysis.Document,
	pos protocol.Position,
) (*protocol.Range, error) {
	target := TargetAt(v, doc, pos)

	switch target.Kind {
	case TargetType, TargetRelation, TargetCondition:
	case TargetFileRef, TargetModule, TargetNone:
		return nil, ErrNotRenameable
	}

	if len(declarations(analysis.ScopeFor(v, doc), target)) == 0 {
		return nil, ErrNotDeclared
	}

	rng := target.Range

	return &rng, nil
}

// Rename rewrites every occurrence of the name under the cursor, across the
// model files in scope and the store tests that exercise them.
func Rename(
	v *analysis.View,
	doc *analysis.Document,
	pos protocol.Position,
	newName string,
) (*protocol.WorkspaceEdit, error) {
	if !identifierPattern.MatchString(newName) {
		return nil, fmt.Errorf("%q is not a valid name", newName)
	}

	target := TargetAt(v, doc, pos)

	switch target.Kind {
	case TargetType, TargetRelation, TargetCondition:
	case TargetFileRef, TargetModule, TargetNone:
		return nil, ErrNotRenameable
	}

	scope := analysis.ScopeFor(v, doc)
	if len(declarations(scope, target)) == 0 {
		return nil, ErrNotDeclared
	}

	if conflict := existingName(scope, target, newName); conflict != "" {
		return nil, errors.New(conflict)
	}

	changes := map[protocol.DocumentUri][]protocol.TextEdit{}

	for _, candidate := range relatedDocuments(v, doc) {
		for _, symbol := range Symbols(v, candidate) {
			if !matches(symbol, target) {
				continue
			}

			changes[candidate.URI] = append(changes[candidate.URI], protocol.TextEdit{
				Range:   symbol.Range,
				NewText: newName,
			})
		}
	}

	if len(changes) == 0 {
		return nil, ErrNotDeclared
	}

	return &protocol.WorkspaceEdit{Changes: changes}, nil
}

// existingName reports a collision, since renaming onto a name that is
// already taken silently merges two declarations.
func existingName(scope analysis.Scope, target Target, newName string) string {
	switch target.Kind {
	case TargetType:
		if scope.HasType(newName) {
			return fmt.Sprintf("type %q already exists", newName)
		}

	case TargetRelation:
		for _, owner := range target.OwnerTypes {
			if scope.HasRelation(owner, newName) {
				return fmt.Sprintf("type %q already has a relation %q", owner, newName)
			}
		}

	case TargetCondition:
		if len(scope.ConditionDecls(newName)) > 0 {
			return fmt.Sprintf("condition %q already exists", newName)
		}

	case TargetFileRef, TargetModule, TargetNone:
	}

	return ""
}
