package agent_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/agent"
)

func open(t *testing.T, fixture string) *agent.Workspace {
	t.Helper()

	w, err := agent.Open(filepath.Join("..", "..", "testdata", fixture))
	if err != nil {
		t.Fatalf("opening %s: %v", fixture, err)
	}

	return w
}

func mustContain(t *testing.T, got string, want ...string) {
	t.Helper()

	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func TestCheckReportsNothingForAValidWorkspace(t *testing.T) {
	t.Parallel()

	mustContain(t, open(t, "workspace").Check(""), "No problems.", "2 models")
}

func TestCheckPointsAtTheOffendingWordWithACaret(t *testing.T) {
	t.Parallel()

	got := open(t, "broken").Check("")

	mustContain(t, got,
		"model.fga:9:31 error",
		"the relation `editor` does not exist.",
		"^^^^^^",
		"store.fga.yaml",
		"Did you mean `owner`?",
	)

	// One-based, so a location can be pasted straight into an editor.
	if strings.Contains(got, ":0:") {
		t.Errorf("expected one-based positions, got a zero:\n%s", got)
	}
}

func TestDefinitionResolvesAQualifiedRelation(t *testing.T) {
	t.Parallel()

	got := open(t, "workspace").Definition("document#can_edit")

	mustContain(t, got,
		"Definition of `document#can_edit`",
		"docs.fga:",
		"define can_edit: owner or admin from organization",
	)
}

func TestDefinitionOfABareRelationNameOffersItsTypes(t *testing.T) {
	t.Parallel()

	// An agent quotes `can_edit`, not `document#can_edit`; the answer has to
	// bridge that rather than report a miss.
	got := open(t, "workspace").Definition("can_edit")

	mustContain(t, got, "is a relation on", "document#can_edit")
}

func TestUnknownNameSuggestsTheNearest(t *testing.T) {
	t.Parallel()

	mustContain(t, open(t, "workspace").Describe("documnt"), "not in this model", "document")
}

func TestReferencesReachStoreTests(t *testing.T) {
	t.Parallel()

	got := open(t, "workspace").References("document#can_edit")

	mustContain(t, got, "References to `document#can_edit`", "docs.fga", "docs.fga.yaml")
}

func TestDescribeMergesExtendedTypes(t *testing.T) {
	t.Parallel()

	got := open(t, "workspace").Describe("organization")

	mustContain(t, got,
		"type organization",
		"core.fga",
		"docs.fga (extend)",
		"can_create_document",
		"member",
	)
}

func TestOverviewNamesModulesAndCounts(t *testing.T) {
	t.Parallel()

	mustContain(t, open(t, "workspace").Overview(),
		"module core", "module docs", "document", "relations")
}

func TestSearchMatchesLoosely(t *testing.T) {
	t.Parallel()

	mustContain(t, open(t, "workspace").Search("cnedt"), "can_edit")
}

func TestRunRereadsTheWorkspace(t *testing.T) {
	t.Parallel()

	// The shell commands are one-shot processes, but MCP is not: a workspace
	// answering a second query must not answer from a stale scan.
	w := open(t, "workspace")

	first := w.Run(func(w *agent.Workspace, arg string) string { return w.Check(arg) }, "")
	second := w.Run(func(w *agent.Workspace, arg string) string { return w.Check(arg) }, "")

	if first != second {
		t.Errorf("a repeated query changed answer:\n%s\n---\n%s", first, second)
	}

	mustContain(t, second, "No problems.")
}
