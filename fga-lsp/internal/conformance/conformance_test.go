// Package conformance checks both halves of this project against the corpus
// the reference Go, JS and Java implementations are tested against.
//
// The corpus is vendored under testdata/upstream; see the README there.
package conformance_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
	"gopkg.in/yaml.v3"

	"github.com/mrocholl/fga-lsp/internal/analysis"
	"github.com/mrocholl/fga-lsp/internal/diag"
)

const corpus = "../../testdata/upstream"

// aheadOfTheServer lists cases the corpus calls valid but the pinned
// `openfga` server still rejects. They are not failures of this project: the
// corpus tracks openfga/language, which moves ahead of the server release the
// CLI embeds.
//
// Each was confirmed by hand against the real binary. `fga v0.8.0` reports,
// word for word, what this server reports:
//
//	$ fga model validate --file inline.fga
//	Error: validation error - validate: condition $expression is undefined for relation viewer
//
//	$ fga model validate --file cycle.fga
//	Error: validation error - validate: the definition of relation 'exclusion1'
//	       in object type 'docs' is invalid: an authorization model cannot contain a cycle
//
// Agreeing with the CLI the user actually runs is the point; drop an entry
// here when bumping `openfga` makes it disagree.
var aheadOfTheServer = map[string]string{
	"inline expression on userset is valid":                                      "$expression not yet known to openfga v1.20.0",
	"inline expression mixed with plain and named condition is valid":            "$expression not yet known to openfga v1.20.0",
	"union does not require all child to have entry":                             "cycle rule relaxed after openfga v1.20.0",
	"union does not require all child to have entry even for exclusion child":    "cycle rule relaxed after openfga v1.20.0",
	"union does not require all child to have entry even for intersection child": "cycle rule relaxed after openfga v1.20.0",
}

type startEnd struct {
	Start int `yaml:"start"`
	End   int `yaml:"end"`
}

type expectedError struct {
	Msg      string   `yaml:"msg"`
	Line     startEnd `yaml:"line"`
	Column   startEnd `yaml:"column"`
	File     string   `yaml:"file"`
	Metadata struct {
		Symbol    string `yaml:"symbol"`
		ErrorType string `yaml:"errorType"`
	} `yaml:"metadata"`
}

type dslCase struct {
	Name           string          `yaml:"name"`
	DSL            string          `yaml:"dsl"`
	ExpectedErrors []expectedError `yaml:"expected_errors"`
	Skip           bool            `yaml:"skip"`
}

func loadCases(t *testing.T, name string) []dslCase {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(corpus, name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}

	var cases []dslCase
	if err := yaml.Unmarshal(content, &cases); err != nil {
		t.Fatalf("parsing %s: %v", name, err)
	}

	if len(cases) == 0 {
		t.Fatalf("%s holds no cases", name)
	}

	return cases
}

// analyze runs one standalone model through the whole pipeline, the way the
// server does for a file that no fga.mod claims.
func analyze(dsl string) []protocol.Diagnostic {
	index := analysis.NewIndex()
	uri := analysis.URIFromPath("/conformance/model.fga")

	index.Put(uri, 1, []byte(dsl))

	var diagnostics []protocol.Diagnostic

	index.Read(func(v *analysis.View) {
		diagnostics = diag.Compute(v, v.Get(uri))[uri]
	})

	return diagnostics
}

// ------------------------------------------------------------------ grammar

// TestGrammarParsesEveryValidModel is the grammar's conformance check: every
// model upstream considers well-formed has to yield a tree with no error
// node, whatever the server then makes of it.
func TestGrammarParsesEveryValidModel(t *testing.T) {
	t.Parallel()

	var files []string

	for _, pattern := range []string{"models/*.fga", "transformer-module/*/module/*.fga", "transformer-module/*/*.fga"} {
		matched, err := filepath.Glob(filepath.Join(corpus, pattern))
		if err != nil {
			t.Fatalf("globbing %s: %v", pattern, err)
		}

		files = append(files, matched...)
	}

	if len(files) < 29 {
		t.Fatalf("expected the vendored corpus to hold at least 29 models, found %d", len(files))
	}

	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()

			// One upstream fixture is deliberately malformed, to exercise the
			// transformer's error reporting.
			if strings.Contains(path, "syntax-error") {
				t.Skip("upstream fixture is intentionally broken")
			}

			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading: %v", err)
			}

			doc := analysis.Analyze(analysis.URIFromPath(path), 1, content)
			defer doc.Close()

			if root := doc.Root(); root == nil || root.HasError() {
				t.Errorf("failed to parse:\n%s", root.ToSexp())
			}
		})
	}
}

// TestGrammarParsesEverySemanticCase covers the other direction: the semantic
// cases are all syntactically valid by construction, including the ones with
// wildly misplaced whitespace, so the grammar must not choke on any of them.
func TestGrammarParsesEverySemanticCase(t *testing.T) {
	t.Parallel()

	for _, testCase := range loadCases(t, "dsl-semantic-validation-cases.yaml") {
		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()

			doc := analysis.Analyze(analysis.URIFromPath("/conformance/model.fga"), 1, []byte(testCase.DSL))
			defer doc.Close()

			if root := doc.Root(); root == nil || root.HasError() {
				t.Errorf("failed to parse:\n%s\n%s", testCase.DSL, root.ToSexp())
			}
		})
	}
}

// ------------------------------------------------------------- server verdict

// TestSyntaxCasesAgree checks that the server calls a model broken exactly
// when upstream does.
func TestSyntaxCasesAgree(t *testing.T) {
	t.Parallel()

	for _, testCase := range loadCases(t, "dsl-syntax-validation-cases.yaml") {
		if testCase.Skip {
			continue
		}

		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()

			if reason, ok := aheadOfTheServer[testCase.Name]; ok {
				t.Skip(reason)
			}

			diagnostics := analyze(testCase.DSL)

			wantErrors := len(testCase.ExpectedErrors) > 0

			switch {
			case wantErrors && len(diagnostics) == 0:
				t.Errorf("expected %d error(s), got none:\n%s", len(testCase.ExpectedErrors), testCase.DSL)
			case !wantErrors && len(diagnostics) > 0:
				t.Errorf("expected no errors, got %v:\n%s", messages(diagnostics), testCase.DSL)
			}
		})
	}
}

func TestSemanticCasesAgree(t *testing.T) {
	t.Parallel()

	for _, testCase := range loadCases(t, "dsl-semantic-validation-cases.yaml") {
		if testCase.Skip {
			continue
		}

		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()

			if reason, ok := aheadOfTheServer[testCase.Name]; ok {
				t.Skip(reason)
			}

			diagnostics := analyze(testCase.DSL)

			if len(testCase.ExpectedErrors) > 0 && len(diagnostics) == 0 {
				t.Errorf("expected %d error(s), got none:\n%s", len(testCase.ExpectedErrors), testCase.DSL)
			}

			if len(testCase.ExpectedErrors) == 0 && len(diagnostics) > 0 {
				t.Errorf("expected no errors, got %v:\n%s", messages(diagnostics), testCase.DSL)
			}
		})
	}
}

// TestSemanticCasePositions holds the server to upstream's exact wording and
// range for the checks it implements itself. The error types left out are the
// ones delegated to openfga's typesystem, which reports them in its own words
// and without a source position.
func TestSemanticCasePositions(t *testing.T) {
	t.Parallel()

	owned := map[string]bool{
		"missing-definition":           true,
		"invalid-type":                 true,
		"invalid-relation-type":        true,
		"invalid-relation-on-tupleset": true,
		"condition-not-defined":        true,
		"condition-not-used":           true,
		"reserved-type-keywords":       true,
		"reserved-relation-keywords":   true,
		"tupleuserset-not-direct":      true,
		"duplicated-error":             true,
		"invalid-name":                 true,
		"schema-version-required":      true,
		"invalid-schema":               true,
	}

	for _, testCase := range loadCases(t, "dsl-semantic-validation-cases.yaml") {
		if testCase.Skip {
			continue
		}

		t.Run(testCase.Name, func(t *testing.T) {
			t.Parallel()

			diagnostics := analyze(testCase.DSL)

			for _, want := range testCase.ExpectedErrors {
				if !owned[want.Metadata.ErrorType] {
					continue
				}

				if !found(diagnostics, want) {
					t.Errorf(
						"missing %s at %d:%d-%d: %q\ngot: %v\n%s",
						want.Metadata.ErrorType,
						want.Line.Start, want.Column.Start, want.Column.End,
						want.Msg, describe(diagnostics), testCase.DSL,
					)
				}
			}
		})
	}
}

func found(diagnostics []protocol.Diagnostic, want expectedError) bool {
	for _, d := range diagnostics {
		if d.Message != want.Msg {
			continue
		}

		if int(d.Range.Start.Line) != want.Line.Start || int(d.Range.Start.Character) != want.Column.Start {
			continue
		}

		if int(d.Range.End.Character) != want.Column.End {
			continue
		}

		return true
	}

	return false
}

func messages(diagnostics []protocol.Diagnostic) []string {
	out := make([]string, 0, len(diagnostics))
	for _, d := range diagnostics {
		out = append(out, d.Message)
	}

	return out
}

func describe(diagnostics []protocol.Diagnostic) []string {
	out := make([]string, 0, len(diagnostics))
	for _, d := range diagnostics {
		out = append(out, strings.Join([]string{
			d.Message,
			"@", itoa(int(d.Range.Start.Line)), ":", itoa(int(d.Range.Start.Character)),
			"-", itoa(int(d.Range.End.Character)),
		}, ""))
	}

	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var digits []byte

	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	return string(digits)
}
