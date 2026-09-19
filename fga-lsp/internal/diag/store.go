package diag

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openfga/language/pkg/go/validation"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/mrocholl/fga-lsp/internal/analysis"
)

// storeDiagnostics checks a `.fga.yaml` against the model it points at:
// tuple and check syntax, and whether every type and relation it names is
// actually in that model.
func storeDiagnostics(view *analysis.View, doc *analysis.Document, result Result) {
	store := doc.Store
	if store == nil {
		return
	}

	if store.ParseError != nil {
		result.add(doc.URI, diagnostic(store.ParseError.Range, store.ParseError.Message, protocol.DiagnosticSeverityError))

		return
	}

	names, ok := storeScope(view, doc, result)
	if !ok {
		return
	}

	checker := storeChecker{doc: doc, names: names, result: result}

	for _, tuple := range store.Tuples {
		checker.tuple(tuple)
	}

	for _, test := range store.Tests {
		for _, tuple := range test.Tuples {
			checker.tuple(tuple)
		}

		for _, check := range test.Checks {
			checker.check(check)
		}

		for _, entry := range test.ListObjects {
			checker.listObjects(entry)
		}

		for _, entry := range test.ListUsers {
			checker.listUsers(entry)
		}
	}
}

// storeScope resolves `model_file` to the documents that back this store. It
// reports when the reference does not resolve, and returns ok=false whenever
// there is nothing to check against.
func storeScope(view *analysis.View, doc *analysis.Document, result Result) (scope, bool) {
	store := doc.Store

	if !store.ModelFile.IsSet() {
		// An inline `model:` block is parsed as its own model; without either
		// there is nothing to resolve names against.
		if store.InlineModel == "" {
			return scope{}, false
		}

		inline := analysis.Analyze(doc.URI+"#model", doc.Version, []byte(store.InlineModel))

		return scope{docs: []*analysis.Document{inline}}, true
	}

	if _, err := os.Stat(store.ModelPath); err != nil {
		result.add(doc.URI, diagnostic(
			store.ModelFile.Range,
			"cannot read "+store.ModelFile.Value+": "+err.Error(),
			protocol.DiagnosticSeverityError,
		))

		return scope{}, false
	}

	if filepath.Base(store.ModelPath) == "fga.mod" {
		mod := view.GetByPath(store.ModelPath)
		if mod == nil || mod.Mod == nil {
			return scope{}, false
		}

		var docs []*analysis.Document

		for _, entry := range mod.Mod.Contents {
			if member := view.GetByPath(entry.Path); member != nil {
				docs = append(docs, member)
			}
		}

		if len(docs) == 0 {
			return scope{}, false
		}

		return scope{docs: docs}, true
	}

	model := view.GetByPath(store.ModelPath)
	if model == nil {
		return scope{}, false
	}

	return scope{docs: []*analysis.Document{model}}, true
}

type storeChecker struct {
	doc    *analysis.Document
	names  scope
	result Result
}

func (c storeChecker) report(rng protocol.Range, message string) {
	c.result.add(c.doc.URI, diagnostic(rng, message, protocol.DiagnosticSeverityError))
}

func (c storeChecker) warn(rng protocol.Range, message string) {
	c.result.add(c.doc.URI, diagnostic(rng, message, protocol.DiagnosticSeverityWarning))
}

func (c storeChecker) tuple(tuple analysis.Tuple) {
	objectType := c.object(tuple.Object)
	c.user(tuple.User)

	if !tuple.Relation.IsSet() {
		return
	}

	if !validation.ValidateRelation(tuple.Relation.Value) {
		c.report(tuple.Relation.Range, fmt.Sprintf("%q is not a valid relation name", tuple.Relation.Value))

		return
	}

	if objectType == "" {
		return
	}

	if !c.names.hasRelation(objectType, tuple.Relation.Value) {
		c.report(tuple.Relation.Range, c.unknownRelation(objectType, tuple.Relation.Value))
	}
}

// object validates an `object:` field and returns its type when the type is
// one the model declares.
func (c storeChecker) object(field analysis.Field) string {
	if !field.IsSet() {
		return ""
	}

	if !validation.ValidateObject(field.Value) {
		c.report(field.Range, fmt.Sprintf("%q is not a valid object; expected <type>:<id>", field.Value))

		return ""
	}

	objectType, _ := analysis.SplitObject(field.Value)

	if !c.names.hasType(objectType) {
		c.report(field.Range, c.unknownType(objectType))

		return ""
	}

	return objectType
}

// user validates a `user:` field, which may be an object, a userset
// `type:id#relation`, or a wildcard `type:*`.
func (c storeChecker) user(field analysis.Field) {
	if !field.IsSet() {
		return
	}

	if !validation.ValidateUser(field.Value) {
		c.report(field.Range, fmt.Sprintf("%q is not a valid user", field.Value))

		return
	}

	userType, _, relation := analysis.SplitTupleUser(field.Value)

	if !c.names.hasType(userType) {
		c.report(field.Range, c.unknownType(userType))

		return
	}

	if relation != "" && !c.names.hasRelation(userType, relation) {
		c.report(field.Range, c.unknownRelation(userType, relation))
	}
}

func (c storeChecker) check(check analysis.Check) {
	objectType := c.object(check.Object)

	for _, object := range check.Objects {
		if t := c.object(object); objectType == "" {
			objectType = t
		}
	}

	c.user(check.User)

	for _, user := range check.Users {
		c.user(user)
	}

	if objectType == "" {
		return
	}

	for _, assertion := range check.Assertions {
		if !c.names.hasRelation(objectType, assertion.Relation.Value) {
			c.report(assertion.Relation.Range, c.unknownRelation(objectType, assertion.Relation.Value))
		}
	}
}

func (c storeChecker) listObjects(entry analysis.ListObjects) {
	c.user(entry.User)

	if !entry.Type.IsSet() {
		return
	}

	if !c.names.hasType(entry.Type.Value) {
		c.report(entry.Type.Range, c.unknownType(entry.Type.Value))

		return
	}

	for _, assertion := range entry.Assertions {
		if !c.names.hasRelation(entry.Type.Value, assertion.Value) {
			c.report(assertion.Range, c.unknownRelation(entry.Type.Value, assertion.Value))
		}
	}
}

func (c storeChecker) listUsers(entry analysis.ListUsers) {
	objectType := c.object(entry.Object)

	for _, filter := range entry.UserFilter {
		if filter.IsSet() && !c.names.hasType(filter.Value) {
			c.report(filter.Range, c.unknownType(filter.Value))
		}
	}

	if objectType == "" {
		return
	}

	for _, assertion := range entry.Assertions {
		if !c.names.hasRelation(objectType, assertion.Value) {
			c.report(assertion.Range, c.unknownRelation(objectType, assertion.Value))
		}
	}
}

func (c storeChecker) unknownType(name string) string {
	return fmt.Sprintf("unknown type %q%s", name, suggest(name, c.names.typeNames()))
}

func (c storeChecker) unknownRelation(typeName, relation string) string {
	var candidates []string
	for _, rel := range c.names.relationsOf(typeName) {
		candidates = append(candidates, rel.Name)
	}

	return fmt.Sprintf("type %q has no relation %q%s", typeName, relation, suggest(relation, candidates))
}

// suggest offers the closest known name, which in a model of near-identical
// `can_*` relations is most of the value of the diagnostic.
func suggest(name string, candidates []string) string {
	best, bestDistance := "", len(name)/2+1

	for _, candidate := range candidates {
		if distance := editDistance(name, candidate); distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}

	if best == "" {
		return ""
	}

	return fmt.Sprintf("; did you mean %q?", best)
}

func editDistance(a, b string) int {
	if a == b {
		return 0
	}

	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)

	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {
		current[0] = i

		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			current[j] = min(previous[j]+1, min(current[j-1]+1, previous[j-1]+cost))
		}

		previous, current = current, previous
	}

	return previous[len(b)]
}

// ------------------------------------------------------------------ fga.mod

func modDiagnostics(view *analysis.View, doc *analysis.Document, result Result) {
	mod := doc.Mod
	if mod == nil {
		return
	}

	for _, modErr := range mod.Errors {
		result.add(doc.URI, diagnostic(modErr.Range, modErr.Message, protocol.DiagnosticSeverityError))
	}

	seen := map[string]protocol.Range{}

	for _, entry := range mod.Contents {
		if _, ok := seen[entry.Value]; ok {
			result.add(doc.URI, diagnostic(
				entry.Range,
				fmt.Sprintf("%q is listed twice", entry.Value),
				protocol.DiagnosticSeverityWarning,
			))
		}

		seen[entry.Value] = entry.Range

		if _, err := os.Stat(entry.Path); err != nil {
			result.add(doc.URI, diagnostic(
				entry.Range,
				"cannot read "+entry.Value+": "+err.Error(),
				protocol.DiagnosticSeverityError,
			))

			continue
		}

		if !strings.HasSuffix(entry.Value, ".fga") {
			result.add(doc.URI, diagnostic(
				entry.Range,
				fmt.Sprintf("%q is not a .fga module file", entry.Value),
				protocol.DiagnosticSeverityWarning,
			))
		}

		result.touch(analysis.URIFromPath(entry.Path))
	}

	// Building the set here means a broken module is reported on the module,
	// not only on the fga.mod that happens to be open.
	if len(mod.Contents) == 0 {
		return
	}

	if member := view.GetByPath(mod.Contents[0].Path); member != nil {
		for uri, diags := range Compute(view, member) {
			result[uri] = append(result[uri], diags...)
		}
	}
}
