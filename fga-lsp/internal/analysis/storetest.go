package analysis

import (
	"path/filepath"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
	"gopkg.in/yaml.v3"
)

// Field is a scalar drawn out of a store-test YAML, kept with the range it
// occupies so a diagnostic or a goto can point at it.
type Field struct {
	Value string
	Range protocol.Range
}

func (f Field) IsSet() bool { return f.Value != "" }

// Tuple is one entry under `tuples:`.
type Tuple struct {
	User     Field
	Relation Field
	Object   Field
	Range    protocol.Range
}

// Assertion is one `relation: bool` pair under a check's `assertions:`.
type Assertion struct {
	Relation Field
	Expected bool
}

type Check struct {
	User       Field
	Users      []Field
	Object     Field
	Objects    []Field
	Assertions []Assertion
	Range      protocol.Range
}

type ListObjects struct {
	User       Field
	Type       Field
	Assertions []Field // relation names
	Range      protocol.Range
}

type ListUsers struct {
	Object     Field
	UserFilter []Field // type names
	Assertions []Field // relation names
	Range      protocol.Range
}

type TestCase struct {
	Name        Field
	Tuples      []Tuple
	Checks      []Check
	ListObjects []ListObjects
	ListUsers   []ListUsers
	Range       protocol.Range
}

// StoreTest is a parsed `.fga.yaml`.
type StoreTest struct {
	Name      Field
	ModelFile Field
	// ModelPath is ModelFile resolved against the store file's directory.
	ModelPath string
	// InlineModel is the `model: |` block, when the store carries one instead
	// of pointing at a file. Line is where the block's first line sits.
	InlineModel     string
	InlineModelLine int

	Tuples []Tuple
	Tests  []TestCase

	// ParseError is set when the YAML itself does not load.
	ParseError *ModError
}

func ParseStoreTest(doc *Document) *StoreTest {
	store := &StoreTest{}

	var root yaml.Node
	if err := yaml.Unmarshal(doc.Content, &root); err != nil {
		store.ParseError = &ModError{
			Message: err.Error(),
			Range:   doc.Lines.LineRange(0),
		}

		return store
	}

	if len(root.Content) == 0 {
		return store
	}

	doc.parseStoreMapping(store, root.Content[0])

	if store.ModelFile.IsSet() {
		store.ModelPath = filepath.Join(filepath.Dir(doc.Path), store.ModelFile.Value)
	}

	return store
}

func (d *Document) parseStoreMapping(store *StoreTest, mapping *yaml.Node) {
	forEachPair(mapping, func(key string, value *yaml.Node) {
		switch key {
		case "name":
			store.Name = d.field(value)
		case "model_file":
			store.ModelFile = d.field(value)
		case "model":
			store.InlineModel = value.Value
			store.InlineModelLine = value.Line - 1
		case "tuples":
			store.Tuples = d.tuples(value)
		case "tests":
			store.Tests = d.testCases(value)
		}
	})
}

func (d *Document) testCases(seq *yaml.Node) []TestCase {
	var out []TestCase

	for _, item := range sequence(seq) {
		testCase := TestCase{Range: d.nodeRange(item)}

		forEachPair(item, func(key string, value *yaml.Node) {
			switch key {
			case "name":
				testCase.Name = d.field(value)
			case "tuples":
				testCase.Tuples = d.tuples(value)
			case "check":
				testCase.Checks = d.checks(value)
			case "list_objects":
				testCase.ListObjects = d.listObjects(value)
			case "list_users":
				testCase.ListUsers = d.listUsers(value)
			}
		})

		out = append(out, testCase)
	}

	return out
}

func (d *Document) tuples(seq *yaml.Node) []Tuple {
	var out []Tuple

	for _, item := range sequence(seq) {
		tuple := Tuple{Range: d.nodeRange(item)}

		forEachPair(item, func(key string, value *yaml.Node) {
			switch key {
			case "user":
				tuple.User = d.field(value)
			case "relation":
				tuple.Relation = d.field(value)
			case "object":
				tuple.Object = d.field(value)
			}
		})

		out = append(out, tuple)
	}

	return out
}

func (d *Document) checks(seq *yaml.Node) []Check {
	var out []Check

	for _, item := range sequence(seq) {
		check := Check{Range: d.nodeRange(item)}

		forEachPair(item, func(key string, value *yaml.Node) {
			switch key {
			case "user":
				check.User = d.field(value)
			case "users":
				check.Users = d.fields(value)
			case "object":
				check.Object = d.field(value)
			case "objects":
				check.Objects = d.fields(value)
			case "assertions":
				forEachPair(value, func(relation string, expected *yaml.Node) {
					check.Assertions = append(check.Assertions, Assertion{
						Relation: Field{Value: relation, Range: d.keyRange(value, relation)},
						Expected: expected.Value == "true",
					})
				})
			}
		})

		out = append(out, check)
	}

	return out
}

func (d *Document) listObjects(seq *yaml.Node) []ListObjects {
	var out []ListObjects

	for _, item := range sequence(seq) {
		entry := ListObjects{Range: d.nodeRange(item)}

		forEachPair(item, func(key string, value *yaml.Node) {
			switch key {
			case "user":
				entry.User = d.field(value)
			case "type":
				entry.Type = d.field(value)
			case "assertions":
				forEachPair(value, func(relation string, _ *yaml.Node) {
					entry.Assertions = append(entry.Assertions, Field{
						Value: relation,
						Range: d.keyRange(value, relation),
					})
				})
			}
		})

		out = append(out, entry)
	}

	return out
}

func (d *Document) listUsers(seq *yaml.Node) []ListUsers {
	var out []ListUsers

	for _, item := range sequence(seq) {
		entry := ListUsers{Range: d.nodeRange(item)}

		forEachPair(item, func(key string, value *yaml.Node) {
			switch key {
			case "object":
				entry.Object = d.field(value)
			case "user_filter":
				for _, filter := range sequence(value) {
					forEachPair(filter, func(filterKey string, filterValue *yaml.Node) {
						if filterKey == "type" {
							entry.UserFilter = append(entry.UserFilter, d.field(filterValue))
						}
					})
				}
			case "assertions":
				forEachPair(value, func(relation string, _ *yaml.Node) {
					entry.Assertions = append(entry.Assertions, Field{
						Value: relation,
						Range: d.keyRange(value, relation),
					})
				})
			}
		})

		out = append(out, entry)
	}

	return out
}

// ---------------------------------------------------------------- yaml glue

func forEachPair(mapping *yaml.Node, fn func(key string, value *yaml.Node)) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		fn(mapping.Content[i].Value, mapping.Content[i+1])
	}
}

func sequence(node *yaml.Node) []*yaml.Node {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}

	return node.Content
}

func (d *Document) field(node *yaml.Node) Field {
	if node == nil {
		return Field{}
	}

	return Field{Value: node.Value, Range: d.scalarRange(node)}
}

func (d *Document) fields(seq *yaml.Node) []Field {
	var out []Field

	for _, item := range sequence(seq) {
		out = append(out, d.field(item))
	}

	return out
}

// scalarRange locates a scalar's text on its line. yaml.v3 reports a column,
// but it counts differently across quoting styles, so the value is searched
// for instead; that also lands inside the quotes rather than on them.
func (d *Document) scalarRange(node *yaml.Node) protocol.Range {
	line := node.Line - 1
	if line < 0 {
		return d.Lines.LineRange(0)
	}

	if node.Value == "" {
		return d.Lines.LineRange(line)
	}

	return valueRange(d.Lines, line, node.Value)
}

// keyRange finds a mapping key inside its own mapping node.
func (d *Document) keyRange(mapping *yaml.Node, key string) protocol.Range {
	if mapping == nil {
		return protocol.Range{}
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return d.scalarRange(mapping.Content[i])
		}
	}

	return d.nodeRange(mapping)
}

func (d *Document) nodeRange(node *yaml.Node) protocol.Range {
	if node == nil {
		return protocol.Range{}
	}

	start := max(node.Line-1, 0)

	end := start
	if node.Kind == yaml.MappingNode || node.Kind == yaml.SequenceNode {
		end = lastLine(node) - 1
	}

	return protocol.Range{
		Start: d.Lines.LineRange(start).Start,
		End:   d.Lines.LineRange(max(end, start)).End,
	}
}

func lastLine(node *yaml.Node) int {
	line := node.Line

	for _, child := range node.Content {
		if childLine := lastLine(child); childLine > line {
			line = childLine
		}
	}

	return line
}

// SplitTupleUser breaks `user:alice#member` into its object and optional
// relation halves. A wildcard `user:*` keeps the `*` as the id.
func SplitTupleUser(user string) (objectType, objectID, relation string) {
	rest := user

	if hash := strings.Index(rest, "#"); hash >= 0 {
		relation = rest[hash+1:]
		rest = rest[:hash]
	}

	if colon := strings.Index(rest, ":"); colon >= 0 {
		return rest[:colon], rest[colon+1:], relation
	}

	return rest, "", relation
}

// SplitObject breaks `document:roadmap` into its type and id.
func SplitObject(object string) (objectType, objectID string) {
	if colon := strings.Index(object, ":"); colon >= 0 {
		return object[:colon], object[colon+1:]
	}

	return object, ""
}
