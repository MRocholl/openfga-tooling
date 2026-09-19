package analysis_test

import (
	"path/filepath"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/mrocholl/fga-lsp/internal/analysis"
)

func TestPathFromURIDoesNotDoubleUnescape(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"/tmp/100%.fga", "/tmp/a b.fga", "/tmp/plain.fga", "/tmp/a%20b.fga"} {
		uri := analysis.URIFromPath(path)

		if got := analysis.PathFromURI(uri); got != filepath.Clean(path) {
			t.Errorf("round trip of %q via %q gave %q", path, uri, got)
		}
	}
}

func TestLeadingCommentIgnoresATrailingOne(t *testing.T) {
	t.Parallel()

	doc := analysis.Analyze(analysis.URIFromPath("/t/m.fga"), 1, []byte(`module m

type document
  relations
    # documents a
    define a: [user] # trailing note about a
    define b: [user]
`))
	defer doc.Close()

	relations := doc.Types[0].Relations

	if got := relations[0].Doc; got != "documents a" {
		t.Errorf("relation a: expected its own doc comment, got %q", got)
	}

	if got := relations[1].Doc; got != "" {
		t.Errorf("relation b: expected no doc comment, got %q", got)
	}
}

func TestStoreTestRangesUseTheReportedColumn(t *testing.T) {
	t.Parallel()

	// `read` also occurs inside `can_read` earlier on the same line; the
	// range has to land on the key that is actually being described.
	doc := analysis.Analyze(analysis.URIFromPath("/t/s.fga.yaml"), 1, []byte(`name: s
tests:
  - name: t
    check:
      - user: user:a
        object: doc:1
        assertions: {can_read: true, read: false}
`))
	defer doc.Close()

	assertions := doc.Store.Tests[0].Checks[0].Assertions
	if len(assertions) != 2 {
		t.Fatalf("expected 2 assertions, got %d", len(assertions))
	}

	first, second := assertions[0], assertions[1]

	if first.Relation.Value != "can_read" || second.Relation.Value != "read" {
		t.Fatalf("unexpected assertion order: %q, %q", first.Relation.Value, second.Relation.Value)
	}

	if second.Relation.Range.Start.Character <= first.Relation.Range.End.Character {
		t.Errorf(
			"the range for %q (%d) overlaps the one for %q (ends at %d)",
			second.Relation.Value, second.Relation.Range.Start.Character,
			first.Relation.Value, first.Relation.Range.End.Character,
		)
	}

	if !first.Expected || second.Expected {
		t.Errorf("assertion values read wrong: %v, %v", first.Expected, second.Expected)
	}
}

func TestYAMLBooleanSpellings(t *testing.T) {
	t.Parallel()

	doc := analysis.Analyze(analysis.URIFromPath("/t/s.fga.yaml"), 1, []byte(`name: s
tests:
  - name: t
    check:
      - user: user:a
        object: doc:1
        assertions:
          a: yes
          b: "true"
          c: off
`))
	defer doc.Close()

	want := map[string]bool{"a": true, "b": true, "c": false}

	for _, assertion := range doc.Store.Tests[0].Checks[0].Assertions {
		if got := assertion.Expected; got != want[assertion.Relation.Value] {
			t.Errorf("%s: expected %v, got %v", assertion.Relation.Value, want[assertion.Relation.Value], got)
		}
	}
}

func TestPutDoesNotFreeATreeStillInUse(t *testing.T) {
	t.Parallel()

	index := analysis.NewIndex()
	uri := analysis.URIFromPath("/t/m.fga")

	index.Put(uri, 1, []byte("module m\ntype user\n"))
	index.Put(uri, 2, []byte("module m\ntype user\ntype org\n"))

	index.Read(func(v *analysis.View) {
		doc := v.Get(uri)
		if doc == nil {
			t.Fatal("document vanished")
		}

		if root := doc.Root(); root == nil {
			t.Fatal("the tree was freed under the stored document")
		}

		if len(doc.Types) != 2 {
			t.Errorf("expected the second version to be stored, got %d types", len(doc.Types))
		}
	})
}

func TestPutIgnoresAStaleVersion(t *testing.T) {
	t.Parallel()

	index := analysis.NewIndex()
	uri := analysis.URIFromPath("/t/m.fga")

	index.Put(uri, 5, []byte("module m\ntype user\ntype org\n"))
	index.Put(uri, 2, []byte("module m\ntype user\n"))

	index.Read(func(v *analysis.View) {
		if got := len(v.Get(uri).Types); got != 2 {
			t.Errorf("a version 2 edit overwrote version 5: %d types", got)
		}
	})
}

func TestInlineModelIsReadAsAModel(t *testing.T) {
	t.Parallel()

	doc := analysis.AnalyzeAs(
		protocol.DocumentUri("file:///t/s.fga.yaml#model"),
		1,
		[]byte("model\n  schema 1.1\ntype user\n"),
		analysis.KindModel,
	)
	defer doc.Close()

	if len(doc.Types) != 1 || doc.Types[0].Name != "user" {
		t.Errorf("expected the inline DSL to be parsed as a model, got %d types", len(doc.Types))
	}
}
