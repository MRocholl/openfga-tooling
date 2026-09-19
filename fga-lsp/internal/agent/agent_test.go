package agent_test

import (
	"encoding/json"
	"path/filepath"
	"sort"
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

	mustContain(t, got, "is a relation", "document#can_edit")
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

// ---------------------------------------------------------------- formats

func TestAgentFormatIsOneFindingPerLine(t *testing.T) {
	t.Parallel()

	got := agent.RenderCheck(open(t, "broken").CheckWorkspace(""), agent.FormatAgent)

	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if strings.HasSuffix(line, "problems") {
			continue
		}

		// path:line:column: severity: message, the shape a compiler emits and
		// every editor and agent already parses.
		if parts := strings.SplitN(line, ": ", 3); len(parts) != 3 ||
			strings.Count(parts[0], ":") != 2 {
			t.Errorf("not in path:line:col: severity: message form: %q", line)
		}
	}

	mustContain(t, got, "model.fga:9:31: error: the relation `editor` does not exist.", "5 problems")
}

func TestAgentReferencesAreFlatCompleteAndLabelled(t *testing.T) {
	t.Parallel()

	result := open(t, "workspace").FindReferences("document#can_edit", 0)
	got := agent.RenderReferences(result, agent.FormatAgent)

	if result.Omitted != 0 {
		t.Errorf("expected nothing dropped without a limit, %d omitted", result.Omitted)
	}

	// Kind is what decides whether an agent may edit a line: a definition is
	// the thing itself, a test is an assertion about it.
	mustContain(t, got, ": definition: ", ": use: ", ": test: ")

	lines := strings.Split(strings.TrimSpace(got), "\n")
	if summary := lines[len(lines)-1]; !strings.Contains(summary, "references in") {
		t.Errorf("expected a count on the last line, got %q", summary)
	}

	// Sorted by path then line, so two runs are diffable.
	if !sort.SliceIsSorted(result.References, func(i, j int) bool {
		a, b := result.References[i], result.References[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}

		return a.Line < b.Line
	}) {
		t.Error("references are not in a stable order")
	}
}

func TestLimitReportsWhatItDropped(t *testing.T) {
	t.Parallel()

	full := open(t, "workspace").FindReferences("document#can_edit", 0)

	limited := open(t, "workspace").FindReferences("document#can_edit", 1)
	if len(limited.References) != 1 {
		t.Fatalf("expected 1 reference, got %d", len(limited.References))
	}

	if want := full.Total - 1; limited.Omitted != want {
		t.Errorf("expected %d omitted, got %d", want, limited.Omitted)
	}

	mustContain(t, agent.RenderReferences(limited, agent.FormatAgent), "omitted by --limit")
}

func TestJSONFormatRoundTrips(t *testing.T) {
	t.Parallel()

	raw := agent.RenderCheck(open(t, "broken").CheckWorkspace(""), agent.FormatJSON)

	var decoded agent.CheckResult
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, raw)
	}

	if len(decoded.Problems) != 5 {
		t.Errorf("expected 5 problems, got %d", len(decoded.Problems))
	}

	first := decoded.Problems[0]
	if first.Line < 1 || first.Column < 1 {
		t.Errorf("expected one-based positions, got %d:%d", first.Line, first.Column)
	}
}

func TestParseFormatRejectsUnknown(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"text", "agent", "json"} {
		if _, ok := agent.ParseFormat(name); !ok {
			t.Errorf("%s should be a format", name)
		}
	}

	if _, ok := agent.ParseFormat("yaml"); ok {
		t.Error("yaml should not be a format")
	}
}
