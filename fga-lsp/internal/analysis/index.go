package analysis

import (
	"os"
	"path/filepath"
	"sort"
	"sync"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// Index holds every file the server knows about.
type Index struct {
	mu     sync.RWMutex
	docs   map[protocol.DocumentUri]*Document
	byPath map[string]*Document
}

func NewIndex() *Index {
	return &Index{
		docs:   make(map[protocol.DocumentUri]*Document),
		byPath: make(map[string]*Document),
	}
}

// View is a read-locked window onto the index.
type View struct {
	docs   map[protocol.DocumentUri]*Document
	byPath map[string]*Document
}

func (ix *Index) Read(fn func(v *View)) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()

	fn(&View{docs: ix.docs, byPath: ix.byPath})
}

// Put analyses content and stores it, replacing whatever was there.
func (ix *Index) Put(uri protocol.DocumentUri, version protocol.Integer, content []byte) {
	doc := Analyze(uri, version, content)

	ix.mu.Lock()
	defer ix.mu.Unlock()

	if previous, ok := ix.docs[uri]; ok {
		if version > 0 && previous.Version > version {
			doc.Close()

			return
		}

		previous.Close()
	}

	ix.docs[uri] = doc

	if doc.Path != "" {
		ix.byPath[doc.Path] = doc
	}
}

func (ix *Index) Delete(uri protocol.DocumentUri) {
	ix.mu.Lock()
	defer ix.mu.Unlock()

	doc, ok := ix.docs[uri]
	if !ok {
		return
	}

	if doc.Path != "" && ix.byPath[doc.Path] == doc {
		delete(ix.byPath, doc.Path)
	}

	doc.Close()
	delete(ix.docs, uri)
}

// EnsureReferences loads referenced files the index has not seen, transitively. See docs/resolution.md.
func (ix *Index) EnsureReferences(uri protocol.DocumentUri) {
	const maxRounds = 8

	if !ix.has(uri) {
		return
	}

	for range maxRounds {
		missing := ix.unresolved()
		if len(missing) == 0 {
			return
		}

		loaded := false

		for _, path := range missing {
			content, err := os.ReadFile(path) //nolint:gosec // paths come from the workspace
			if err != nil {
				continue
			}

			ix.Put(URIFromPath(path), 0, content)

			loaded = true
		}

		if !loaded {
			return
		}
	}
}

func (ix *Index) has(uri protocol.DocumentUri) bool {
	ix.mu.RLock()
	defer ix.mu.RUnlock()

	_, ok := ix.docs[uri]

	return ok
}

// unresolved collects what every known document still needs.
func (ix *Index) unresolved() []string {
	ix.mu.RLock()
	defer ix.mu.RUnlock()

	view := &View{docs: ix.docs, byPath: ix.byPath}

	seen := map[string]struct{}{}

	var missing []string

	for _, doc := range ix.docs {
		for _, path := range view.unresolvedReferences(doc) {
			if _, ok := seen[path]; ok {
				continue
			}

			seen[path] = struct{}{}

			missing = append(missing, path)
		}
	}

	return missing
}

// unresolvedReferences lists the on-disk files doc depends on that are not in the index yet.
func (v *View) unresolvedReferences(doc *Document) []string {
	var wanted []string

	switch doc.Kind {
	case KindModFile:
		if doc.Mod != nil {
			for _, entry := range doc.Mod.Contents {
				wanted = append(wanted, entry.Path)
			}
		}

	case KindStoreTest:
		if doc.Store != nil && doc.Store.ModelPath != "" {
			wanted = append(wanted, doc.Store.ModelPath)
		}

	case KindModel:
		if path := findModFile(doc.Path); path != "" {
			wanted = append(wanted, path)
		}

	case KindUnknown:
	}

	var missing []string

	for _, path := range wanted {
		if path == "" || v.GetByPath(path) != nil {
			continue
		}

		missing = append(missing, path)
	}

	return missing
}

// findModFile walks up from a module looking for the fga.mod that lists it.
func findModFile(path string) string {
	const maxLevels = 4

	dir := filepath.Dir(path)

	for range maxLevels {
		candidate := filepath.Join(dir, "fga.mod")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	return ""
}

// Analyze parses and extracts a single document, choosing how to read it from its file name.
func Analyze(uri protocol.DocumentUri, version protocol.Integer, content []byte) *Document {
	return AnalyzeAs(uri, version, content, KindOf(PathFromURI(uri)))
}

// AnalyzeAs is Analyze for content whose kind the file name does not give away.
func AnalyzeAs(uri protocol.DocumentUri, version protocol.Integer, content []byte, kind Kind) *Document {
	path := PathFromURI(uri)

	doc := &Document{
		URI:     uri,
		Path:    path,
		Kind:    kind,
		Version: version,
		Content: content,
		Lines:   NewLineIndex(content),
	}

	switch doc.Kind {
	case KindModel:
		doc.Tree = Parse(content, nil)
		Extract(doc)
	case KindStoreTest:
		doc.Store = ParseStoreTest(doc)
	case KindModFile:
		doc.Mod = ParseModFile(doc)
	case KindUnknown:
	}

	return doc
}

func (v *View) Get(uri protocol.DocumentUri) *Document { return v.docs[uri] }

func (v *View) GetByPath(path string) *Document {
	return v.byPath[filepath.Clean(path)]
}

func (v *View) Documents() []*Document {
	out := make([]*Document, 0, len(v.docs))
	for _, doc := range v.docs {
		out = append(out, doc)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })

	return out
}

// ModelDocs returns the `.fga` documents, sorted by path for stable output.
func (v *View) ModelDocs() []*Document {
	var out []*Document

	for _, doc := range v.Documents() {
		if doc.Kind == KindModel {
			out = append(out, doc)
		}
	}

	return out
}

// TypeDecls returns every declaration of a type name, the original and each `extend type`, in workspace order.
func (v *View) TypeDecls(name string) []*TypeDecl {
	var out []*TypeDecl

	for _, doc := range v.ModelDocs() {
		for _, decl := range doc.Types {
			if decl.Name == name {
				out = append(out, decl)
			}
		}
	}

	return out
}

// TypeNames lists every declared type name, deduplicated.
func (v *View) TypeNames() []string {
	seen := map[string]struct{}{}

	var out []string

	for _, doc := range v.ModelDocs() {
		for _, decl := range doc.Types {
			if _, ok := seen[decl.Name]; ok {
				continue
			}

			seen[decl.Name] = struct{}{}

			out = append(out, decl.Name)
		}
	}

	sort.Strings(out)

	return out
}

// RelationDecls returns every `define` of relation on typeName.
func (v *View) RelationDecls(typeName, relation string) []*RelationDecl {
	var out []*RelationDecl

	for _, decl := range v.TypeDecls(typeName) {
		for _, rel := range decl.Relations {
			if rel.Name == relation {
				out = append(out, rel)
			}
		}
	}

	return out
}

// RelationsOfType lists a type's relations across all its declarations.
func (v *View) RelationsOfType(typeName string) []*RelationDecl {
	seen := map[string]struct{}{}

	var out []*RelationDecl

	for _, decl := range v.TypeDecls(typeName) {
		for _, rel := range decl.Relations {
			if _, ok := seen[rel.Name]; ok {
				continue
			}

			seen[rel.Name] = struct{}{}

			out = append(out, rel)
		}
	}

	return out
}

func (v *View) HasType(name string) bool { return len(v.TypeDecls(name)) > 0 }

func (v *View) ConditionDecls(name string) []*ConditionDecl {
	var out []*ConditionDecl

	for _, doc := range v.ModelDocs() {
		for _, cond := range doc.Conds {
			if cond.Name == name {
				out = append(out, cond)
			}
		}
	}

	return out
}

func (v *View) ConditionNames() []string {
	var out []string

	for _, doc := range v.ModelDocs() {
		for _, cond := range doc.Conds {
			out = append(out, cond.Name)
		}
	}

	sort.Strings(out)

	return out
}

// ModuleSetFor returns the fga.mod that lists path, and the model files it names.
func (v *View) ModuleSetFor(path string) (mod *Document, members []string) {
	for _, doc := range v.Documents() {
		if doc.Kind != KindModFile || doc.Mod == nil {
			continue
		}

		for _, entry := range doc.Mod.Contents {
			if sameFile(entry.Path, path) {
				return doc, modMembers(doc)
			}
		}
	}

	return nil, nil
}

// ModFiles lists the fga.mod documents in the workspace.
func (v *View) ModFiles() []*Document {
	var out []*Document

	for _, doc := range v.Documents() {
		if doc.Kind == KindModFile {
			out = append(out, doc)
		}
	}

	return out
}

func modMembers(mod *Document) []string {
	out := make([]string, 0, len(mod.Mod.Contents))
	for _, entry := range mod.Mod.Contents {
		out = append(out, entry.Path)
	}

	return out
}

// Contents returns the text of path, preferring the synced buffer over disk.
func (v *View) Contents(path string) ([]byte, error) {
	if doc := v.GetByPath(path); doc != nil {
		return doc.Content, nil
	}

	return os.ReadFile(path) //nolint:gosec
}

func sameFile(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}
