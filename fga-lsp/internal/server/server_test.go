package server_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/mrocholl/fga-lsp/internal/analysis"
	"github.com/mrocholl/fga-lsp/internal/server"
)

// session drives a server the way an editor would: initialize against a
// directory, then open documents and collect what the server pushes back.
type session struct {
	t        *testing.T
	handler  *protocol.Handler
	ctx      *glsp.Context
	root     string
	reported map[protocol.DocumentUri][]protocol.Diagnostic
}

func newSession(t *testing.T, fixture string) *session {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", fixture))
	if err != nil {
		t.Fatalf("resolving fixture: %v", err)
	}

	s := &session{
		t:        t,
		handler:  server.New("test").Handler(),
		root:     root,
		reported: map[protocol.DocumentUri][]protocol.Diagnostic{},
	}

	s.ctx = &glsp.Context{Notify: func(method string, params any) {
		if method != protocol.ServerTextDocumentPublishDiagnostics {
			return
		}

		published, ok := params.(protocol.PublishDiagnosticsParams)
		if !ok {
			return
		}

		s.reported[published.URI] = published.Diagnostics
	}}

	rootURI := analysis.URIFromPath(root)

	if _, err := s.handler.Initialize(s.ctx, &protocol.InitializeParams{
		RootURI: &rootURI,
	}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	return s
}

// open syncs a fixture file and returns its URI.
func (s *session) open(name string) protocol.DocumentUri {
	s.t.Helper()

	path := filepath.Join(s.root, name)

	content, err := os.ReadFile(path)
	if err != nil {
		s.t.Fatalf("reading %s: %v", name, err)
	}

	uri := analysis.URIFromPath(path)

	if err := s.handler.TextDocumentDidOpen(s.ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: uri, Version: 1, Text: string(content)},
	}); err != nil {
		s.t.Fatalf("didOpen %s: %v", name, err)
	}

	return uri
}

// edit replaces a document's content, as a keystroke would.
func (s *session) edit(uri protocol.DocumentUri, text string) {
	s.t.Helper()

	if err := s.handler.TextDocumentDidChange(s.ctx, &protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uri},
			Version:                2,
		},
		ContentChanges: []any{protocol.TextDocumentContentChangeEventWhole{Text: text}},
	}); err != nil {
		s.t.Fatalf("didChange: %v", err)
	}
}

func (s *session) diagnostics(uri protocol.DocumentUri) []protocol.Diagnostic {
	return s.reported[uri]
}

// messages flattens a document's diagnostics for readable assertions.
func (s *session) messages(uri protocol.DocumentUri) []string {
	out := make([]string, 0, len(s.reported[uri]))
	for _, d := range s.reported[uri] {
		out = append(out, d.Message)
	}

	return out
}

func position(line, character uint32) protocol.Position {
	return protocol.Position{Line: line, Character: character}
}

// ------------------------------------------------------------- diagnostics

func TestValidWorkspaceReportsNothing(t *testing.T) {
	t.Parallel()

	s := newSession(t, "workspace")

	for _, name := range []string{"core.fga", "docs.fga", "docs.fga.yaml", "fga.mod"} {
		uri := s.open(name)

		if got := s.diagnostics(uri); len(got) != 0 {
			t.Errorf("%s: expected no diagnostics, got %v", name, s.messages(uri))
		}
	}
}

func TestUnknownRelationIsReportedOnTheWord(t *testing.T) {
	t.Parallel()

	s := newSession(t, "broken")
	uri := s.open("model.fga")

	// The wording is openfga/language's, so a squiggle here reads the same
	// as `fga model validate` in the terminal. `member from organization`
	// makes `organization` a relation on `document`, not a type reference,
	// so that is how it is reported.
	want := map[string]bool{
		"the relation `editor` does not exist.":                  false,
		"`organization` is not a valid relation for `document`.": false,
	}

	for _, message := range s.messages(uri) {
		if _, ok := want[message]; ok {
			want[message] = true
		}
	}

	for message, seen := range want {
		if !seen {
			t.Errorf("missing diagnostic %q; got %v", message, s.messages(uri))
		}
	}

	// The range has to cover just the offending name, or the squiggle is
	// useless in a line of five relation names.
	for _, d := range s.diagnostics(uri) {
		if d.Message != "the relation `editor` does not exist." {
			continue
		}

		if d.Range.Start.Line != 8 {
			t.Errorf("expected the diagnostic on line 8, got %d", d.Range.Start.Line)
		}

		if width := d.Range.End.Character - d.Range.Start.Character; width != uint32(len("editor")) {
			t.Errorf("expected the range to cover %q, got width %d", "editor", width)
		}
	}
}

func TestStoreTestNamesAreCheckedAgainstTheModel(t *testing.T) {
	t.Parallel()

	s := newSession(t, "broken")
	uri := s.open("store.fga.yaml")

	joined := strings.Join(s.messages(uri), "\n")

	for _, want := range []string{
		`type "document" has no relation "ownr"; did you mean "owner"?`,
		`unknown type "folder"`,
		`has no relation "can_veiw"; did you mean "can_view"?`,
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
}

func TestEditingAModuleRevalidatesItsSiblings(t *testing.T) {
	t.Parallel()

	s := newSession(t, "workspace")

	core := s.open("core.fga")
	docs := s.open("docs.fga")

	if got := s.diagnostics(docs); len(got) != 0 {
		t.Fatalf("expected a clean start, got %v", s.messages(docs))
	}

	// docs.fga reaches `admin` through `organization`; removing it from core
	// must light up docs.fga, not core.fga.
	s.edit(core, strings.Replace(
		mustRead(t, filepath.Join(s.root, "core.fga")),
		"    define admin: [user]\n", "", 1,
	))

	if got := s.messages(docs); len(got) == 0 {
		t.Error("expected docs.fga to break when core.fga loses a relation it uses")
	} else if !strings.Contains(strings.Join(got, "\n"), "`admin`") {
		t.Errorf("expected a diagnostic naming admin, got %v", got)
	}
}

func TestDiagnosticsAreClearedWhenFixed(t *testing.T) {
	t.Parallel()

	s := newSession(t, "broken")
	uri := s.open("model.fga")

	if len(s.diagnostics(uri)) == 0 {
		t.Fatal("expected the fixture to be broken")
	}

	s.edit(uri, `model
  schema 1.1

type user

type document
  relations
    define owner: [user]
    define can_edit: owner
`)

	if got := s.diagnostics(uri); len(got) != 0 {
		t.Errorf("expected diagnostics to clear, got %v", s.messages(uri))
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	return string(content)
}

// TestExternalWorkspace points the server at a real model tree and asserts it
// stays quiet. Synthetic fixtures cannot cover the shapes a production model
// reaches for, so this runs against one on demand:
//
//	FGA_LSP_WORKSPACE=/path/to/authz go test ./internal/server/
func TestExternalWorkspace(t *testing.T) {
	t.Parallel()

	root := os.Getenv("FGA_LSP_WORKSPACE")
	if root == "" {
		t.Skip("set FGA_LSP_WORKSPACE to a directory of .fga files to run this")
	}

	reported := map[protocol.DocumentUri][]protocol.Diagnostic{}

	ctx := &glsp.Context{Notify: func(method string, params any) {
		if method != protocol.ServerTextDocumentPublishDiagnostics {
			return
		}

		if published, ok := params.(protocol.PublishDiagnosticsParams); ok {
			reported[published.URI] = published.Diagnostics
		}
	}}

	handler := server.New("test").Handler()
	rootURI := analysis.URIFromPath(root)

	if _, err := handler.Initialize(ctx, &protocol.InitializeParams{RootURI: &rootURI}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	var opened int

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || analysis.KindOf(path) == analysis.KindUnknown {
			return nil //nolint:nilerr
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil //nolint:nilerr
		}

		opened++

		return handler.TextDocumentDidOpen(ctx, &protocol.DidOpenTextDocumentParams{
			TextDocument: protocol.TextDocumentItem{
				URI:     analysis.URIFromPath(path),
				Version: 1,
				Text:    string(content),
			},
		})
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	t.Logf("opened %d files under %s", opened, root)

	for uri, diagnostics := range reported {
		for _, d := range diagnostics {
			t.Errorf("%s:%d: %s", filepath.Base(analysis.PathFromURI(uri)), d.Range.Start.Line+1, d.Message)
		}
	}
}

// TestInlineModelStoreTestResolves covers a store test that carries its model
// in a `model: |` block instead of pointing at a file. That block has no file
// name, so its kind has to be stated rather than inferred, or every type in
// it reads as undeclared.
func TestInlineModelStoreTestResolves(t *testing.T) {
	t.Parallel()

	s := newSession(t, "inline")
	uri := s.open("store.fga.yaml")

	if got := s.diagnostics(uri); len(got) != 0 {
		t.Errorf("expected no diagnostics, got %v", s.messages(uri))
	}
}

// TestModularSyntaxErrorLandsInItsModule pins down where a syntax error shows
// up. The tree-sitter grammar is looser than the reference parser on purpose,
// so a module can parse cleanly here and still be rejected by `fga`; the
// report has to name the module, not the fga.mod that happens to list it.
func TestModularSyntaxErrorLandsInItsModule(t *testing.T) {
	t.Parallel()

	s := newSession(t, "workspace")

	docs := s.open("docs.fga")
	modURI := analysis.URIFromPath(filepath.Join(s.root, "fga.mod"))

	// Two relations on one line: legal to the grammar, not to ANTLR.
	s.edit(docs, "module docs\n\ntype note\n  relations\n    define a: [user] define b: [user]\n")

	diagnostics := s.diagnostics(docs)
	if len(diagnostics) == 0 {
		t.Fatal("expected the module to be reported as broken")
	}

	if got := s.diagnostics(modURI); len(got) != 0 {
		t.Errorf("fga.mod should be clean, got %v", s.messages(modURI))
	}

	for _, d := range diagnostics {
		if strings.HasPrefix(d.Message, "syntax error at line=") {
			t.Errorf("the raw transformer string leaked into the message: %q", d.Message)
		}

		if d.Range.Start.Line == 0 {
			t.Errorf("expected the error on the offending line, not line 1: %q", d.Message)
		}
	}
}

// TestUnscannedSiblingsAreLoaded covers the client that syncs only the file
// it opens. Without pulling in the rest of the module set, every name defined
// in a sibling resolves to "unknown relation" -- a correct line reported as
// an error.
func TestUnscannedSiblingsAreLoaded(t *testing.T) {
	t.Parallel()

	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "workspace"))
	if err != nil {
		t.Fatalf("resolving fixture: %v", err)
	}

	s := &session{
		t:        t,
		handler:  server.New("test").Handler(),
		root:     root,
		reported: map[protocol.DocumentUri][]protocol.Diagnostic{},
	}

	s.ctx = &glsp.Context{Notify: func(method string, params any) {
		if method != protocol.ServerTextDocumentPublishDiagnostics {
			return
		}

		if published, ok := params.(protocol.PublishDiagnosticsParams); ok {
			s.reported[published.URI] = published.Diagnostics
		}
	}}

	// No rootUri: nothing is scanned up front, so the index starts empty.
	if _, err := s.handler.Initialize(s.ctx, &protocol.InitializeParams{}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// docs.fga reaches `admin` and `user`, both declared in core.fga.
	uri := s.open("docs.fga")

	if got := s.diagnostics(uri); len(got) != 0 {
		t.Errorf("expected the sibling module to be loaded on demand, got %v", s.messages(uri))
	}
}
