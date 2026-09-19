// Package server wires the language features onto the LSP transport.
package server

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/mrocholl/fga-lsp/internal/analysis"
	"github.com/mrocholl/fga-lsp/internal/diag"
	"github.com/mrocholl/fga-lsp/internal/feature"
)

// Name is how the server identifies itself to a client.
const Name = "fga-lsp"

// Server holds the workspace. Everything it answers is derived from the
// index, so a request never reaches the filesystem for a file the editor has
// open.
type Server struct {
	index   *analysis.Index
	version string
	// published remembers which documents currently carry diagnostics, so
	// that a file which becomes clean gets an explicit empty publish rather
	// than keeping stale squiggles.
	published map[protocol.DocumentUri]struct{}
}

func New(version string) *Server {
	return &Server{
		index:     analysis.NewIndex(),
		version:   version,
		published: map[protocol.DocumentUri]struct{}{},
	}
}

// Handler builds the glsp handler with every method this server implements.
func (s *Server) Handler() *protocol.Handler {
	return &protocol.Handler{
		Initialize:  s.initialize,
		Initialized: func(*glsp.Context, *protocol.InitializedParams) error { return nil },
		Shutdown: func(*glsp.Context) error {
			return nil
		},
		SetTrace: func(*glsp.Context, *protocol.SetTraceParams) error { return nil },

		TextDocumentDidOpen:   s.didOpen,
		TextDocumentDidChange: s.didChange,
		TextDocumentDidSave:   s.didSave,
		TextDocumentDidClose:  s.didClose,

		TextDocumentDefinition:     s.definition,
		TextDocumentHover:          s.hover,
		TextDocumentReferences:     s.references,
		TextDocumentDocumentSymbol: s.documentSymbol,
		WorkspaceSymbol:            s.workspaceSymbol,
		TextDocumentCompletion:     s.completion,
		TextDocumentPrepareRename:  s.prepareRename,
		TextDocumentRename:         s.rename,
		TextDocumentFormatting:     s.formatting,
	}
}

func (s *Server) initialize(_ *glsp.Context, params *protocol.InitializeParams) (any, error) {
	for _, root := range roots(params) {
		s.scan(root)
	}

	sync := protocol.TextDocumentSyncKindFull
	trigger := []string{":", "[", "#", ",", " "}
	resolve := false

	capabilities := protocol.ServerCapabilities{
		TextDocumentSync: protocol.TextDocumentSyncOptions{
			OpenClose: boolPtr(true),
			Change:    &sync,
			Save:      protocol.SaveOptions{IncludeText: boolPtr(false)},
		},
		DefinitionProvider:         true,
		HoverProvider:              true,
		ReferencesProvider:         true,
		DocumentSymbolProvider:     true,
		WorkspaceSymbolProvider:    true,
		DocumentFormattingProvider: true,
		CompletionProvider: &protocol.CompletionOptions{
			TriggerCharacters: trigger,
			ResolveProvider:   &resolve,
		},
		RenameProvider: &protocol.RenameOptions{PrepareProvider: boolPtr(true)},
	}

	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    Name,
			Version: &s.version,
		},
	}, nil
}

// roots collects the directories to scan, preferring workspace folders and
// falling back to the deprecated root fields that some clients still send.
func roots(params *protocol.InitializeParams) []string {
	var out []string

	for _, folder := range params.WorkspaceFolders {
		if path := analysis.PathFromURI(protocol.DocumentUri(folder.URI)); path != "" {
			out = append(out, path)
		}
	}

	if len(out) > 0 {
		return out
	}

	if params.RootURI != nil {
		if path := analysis.PathFromURI(*params.RootURI); path != "" {
			return []string{path}
		}
	}

	if params.RootPath != nil && *params.RootPath != "" {
		return []string{*params.RootPath}
	}

	return nil
}

// scan loads every model, store test and fga.mod under root. Cross-file
// features need the whole set before the first file is opened.
func (s *Server) scan(root string) {
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable subtree is not fatal
		}

		if entry.IsDir() {
			if skipDir(entry.Name()) {
				return fs.SkipDir
			}

			return nil
		}

		if analysis.KindOf(path) == analysis.KindUnknown {
			return nil
		}

		content, err := os.ReadFile(path) //nolint:gosec // paths come from the workspace root
		if err != nil {
			return nil //nolint:nilerr
		}

		s.index.Put(analysis.URIFromPath(path), 0, content)

		return nil
	})
}

func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".venv", "target", "dist", "build":
		return true
	}

	return strings.HasPrefix(name, ".") && name != "."
}

// ------------------------------------------------- document synchronisation

func (s *Server) didOpen(ctx *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	s.index.Put(params.TextDocument.URI, params.TextDocument.Version, []byte(params.TextDocument.Text))

	s.publish(ctx, params.TextDocument.URI)

	return nil
}

func (s *Server) didChange(ctx *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	text, ok := fullText(params.ContentChanges)
	if !ok {
		return nil
	}

	s.index.Put(params.TextDocument.URI, params.TextDocument.Version, []byte(text))

	s.publish(ctx, params.TextDocument.URI)

	return nil
}

func (s *Server) didSave(ctx *glsp.Context, params *protocol.DidSaveTextDocumentParams) error {
	s.publish(ctx, params.TextDocument.URI)

	return nil
}

func (s *Server) didClose(ctx *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	// The file stays in the index, re-read from disk: closing a module must
	// not make its types vanish from the sibling that is still open.
	path := analysis.PathFromURI(params.TextDocument.URI)

	content, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		s.index.Delete(params.TextDocument.URI)

		return nil
	}

	s.index.Put(params.TextDocument.URI, 0, content)

	s.publish(ctx, params.TextDocument.URI)

	return nil
}

// fullText pulls the replacement text out of a didChange. The server
// advertises full synchronisation, so a ranged change is a client bug rather
// than something to apply blindly.
func fullText(changes []any) (string, bool) {
	for _, change := range changes {
		switch typed := change.(type) {
		case protocol.TextDocumentContentChangeEventWhole:
			return typed.Text, true
		case protocol.TextDocumentContentChangeEvent:
			if typed.Range == nil {
				return typed.Text, true
			}
		}
	}

	return "", false
}

// publish recomputes diagnostics for a document and everything that shares
// its model, and clears any file that has become clean.
func (s *Server) publish(ctx *glsp.Context, uri protocol.DocumentUri) {
	results := diag.Result{}

	s.index.Read(func(v *analysis.View) {
		doc := v.Get(uri)
		if doc == nil {
			return
		}

		for target, diags := range diag.Compute(v, doc) {
			results[target] = append(results[target], diags...)
		}

		// A model change can invalidate a store test that never moved, so the
		// tests pointing at this model are recomputed too.
		if doc.Kind != analysis.KindStoreTest {
			for _, store := range storesFor(v, doc) {
				for target, diags := range diag.Compute(v, store) {
					results[target] = append(results[target], diags...)
				}
			}
		}
	})

	for target, diags := range results {
		if len(diags) == 0 {
			if _, ok := s.published[target]; !ok {
				continue
			}

			delete(s.published, target)
		} else {
			s.published[target] = struct{}{}
		}

		ctx.Notify(protocol.ServerTextDocumentPublishDiagnostics, protocol.PublishDiagnosticsParams{
			URI:         target,
			Diagnostics: diags,
		})
	}
}

// storesFor lists the store tests whose model scope includes doc.
func storesFor(v *analysis.View, doc *analysis.Document) []*analysis.Document {
	var out []*analysis.Document

	for _, candidate := range v.Documents() {
		if candidate.Kind != analysis.KindStoreTest {
			continue
		}

		for _, model := range analysis.ScopeFor(v, candidate).Docs {
			if model.URI == doc.URI {
				out = append(out, candidate)

				break
			}
		}
	}

	return out
}

// ------------------------------------------------------------- features

func (s *Server) definition(
	_ *glsp.Context,
	params *protocol.DefinitionParams,
) (any, error) {
	var out []protocol.Location

	s.withDoc(params.TextDocument.URI, func(v *analysis.View, doc *analysis.Document) {
		out = feature.Definition(v, doc, params.Position)
	})

	if len(out) == 0 {
		return nil, nil
	}

	return out, nil
}

func (s *Server) hover(_ *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	var out *protocol.Hover

	s.withDoc(params.TextDocument.URI, func(v *analysis.View, doc *analysis.Document) {
		out = feature.Hover(v, doc, params.Position)
	})

	return out, nil
}

func (s *Server) references(
	_ *glsp.Context,
	params *protocol.ReferenceParams,
) ([]protocol.Location, error) {
	var out []protocol.Location

	s.withDoc(params.TextDocument.URI, func(v *analysis.View, doc *analysis.Document) {
		out = feature.References(v, doc, params.Position, params.Context.IncludeDeclaration)
	})

	return out, nil
}

func (s *Server) documentSymbol(
	_ *glsp.Context,
	params *protocol.DocumentSymbolParams,
) (any, error) {
	var out []protocol.DocumentSymbol

	s.withDoc(params.TextDocument.URI, func(_ *analysis.View, doc *analysis.Document) {
		out = feature.DocumentSymbols(doc)
	})

	if len(out) == 0 {
		return nil, nil
	}

	return out, nil
}

func (s *Server) workspaceSymbol(
	_ *glsp.Context,
	params *protocol.WorkspaceSymbolParams,
) ([]protocol.SymbolInformation, error) {
	var out []protocol.SymbolInformation

	s.index.Read(func(v *analysis.View) {
		out = feature.WorkspaceSymbols(v, params.Query)
	})

	return out, nil
}

func (s *Server) completion(
	_ *glsp.Context,
	params *protocol.CompletionParams,
) (any, error) {
	var out []protocol.CompletionItem

	s.withDoc(params.TextDocument.URI, func(v *analysis.View, doc *analysis.Document) {
		out = feature.Completion(v, doc, params.Position)
	})

	if len(out) == 0 {
		return nil, nil
	}

	return out, nil
}

func (s *Server) prepareRename(
	_ *glsp.Context,
	params *protocol.PrepareRenameParams,
) (any, error) {
	var (
		out *protocol.Range
		err error
	)

	s.withDoc(params.TextDocument.URI, func(v *analysis.View, doc *analysis.Document) {
		out, err = feature.PrepareRename(v, doc, params.Position)
	})

	if err != nil {
		return nil, nil //nolint:nilerr // a refusal is "not renameable", not a failed request
	}

	return out, nil
}

func (s *Server) rename(
	_ *glsp.Context,
	params *protocol.RenameParams,
) (*protocol.WorkspaceEdit, error) {
	var (
		out *protocol.WorkspaceEdit
		err error
	)

	s.withDoc(params.TextDocument.URI, func(v *analysis.View, doc *analysis.Document) {
		out, err = feature.Rename(v, doc, params.Position, params.NewName)
	})

	return out, err
}

func (s *Server) formatting(
	_ *glsp.Context,
	params *protocol.DocumentFormattingParams,
) ([]protocol.TextEdit, error) {
	var out []protocol.TextEdit

	s.withDoc(params.TextDocument.URI, func(_ *analysis.View, doc *analysis.Document) {
		out = feature.Format(doc)
	})

	return out, nil
}

// withDoc runs fn against a document under the index read lock, doing nothing
// when the document is unknown.
func (s *Server) withDoc(uri protocol.DocumentUri, fn func(*analysis.View, *analysis.Document)) {
	s.index.Read(func(v *analysis.View) {
		if doc := v.Get(uri); doc != nil {
			fn(v, doc)
		}
	})
}

func boolPtr(b bool) *bool { return &b }
