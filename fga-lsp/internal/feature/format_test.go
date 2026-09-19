package feature_test

import (
	"strings"
	"testing"

	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/analysis"
	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/feature"
)

func format(t *testing.T, source string) string {
	t.Helper()

	doc := analysis.Analyze(analysis.URIFromPath("/t/m.fga"), 1, []byte(source))
	defer doc.Close()

	edits := feature.Format(doc)
	if len(edits) == 0 {
		return source
	}

	return edits[0].NewText
}

// A blank line inside a relations block separates the capabilities from the
// permissions derived off them. Eating it reflows the author's grouping on
// every save.
func TestFormatKeepsBlankLinesBetweenRelations(t *testing.T) {
	t.Parallel()

	source := `module billing

extend type organization
  relations
    define laboratory_administrate_billing: [role#assignee]
    define laboratory_bill_cases: [role#assignee]

    define can_administrate_billing: laboratory_administrate_billing
`

	if got := format(t, source); got != source {
		t.Errorf("formatting changed already-canonical source:\n--- want\n%s\n--- got\n%s", source, got)
	}
}

func TestFormatIsIdempotent(t *testing.T) {
	t.Parallel()

	source := `module m

type user

type doc
  relations
    # who owns it
    define owner:  [user]

    define can_edit:   owner   or   admin from organization
`

	once := format(t, source)
	if twice := format(t, once); twice != once {
		t.Errorf("a second pass changed the output:\n--- once\n%s\n--- twice\n%s", once, twice)
	}

	if !strings.Contains(once, "define can_edit: owner or admin from organization") {
		t.Errorf("expected operator spacing to be normalised, got:\n%s", once)
	}

	if !strings.Contains(once, "# who owns it") {
		t.Errorf("comment was dropped:\n%s", once)
	}
}

// Formatting a file that does not parse would delete whatever the formatter
// failed to understand.
func TestFormatRefusesABrokenFile(t *testing.T) {
	t.Parallel()

	doc := analysis.Analyze(analysis.URIFromPath("/t/m.fga"), 1, []byte("type doc\n  relations\n    define a: [\n"))
	defer doc.Close()

	if edits := feature.Format(doc); len(edits) != 0 {
		t.Errorf("expected no edits for a broken file, got %d", len(edits))
	}
}
