package analysis

import (
	"os"
	"path/filepath"
	"sort"
	"sync"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// Index holds every file the server knows about. Authorization models are
// small -- tens of files, hundreds of relations -- so queries walk the
// documents rather than maintaining derived maps that would need
// invalidating.
type Index struct {
	mu   sync.RWMutex
	docs map[protocol.DocumentUri]*Document
}

func NewIndex() *Index {
	return &Index{docs: make(map[protocol.DocumentUri]*Document)}
}

// View is a read-locked window onto the index. Every query method lives here
// so that a feature can ask several questions against one consistent state.
type View struct{ docs map[protocol.DocumentUri]*Document }

func (ix *Index) Read(fn func(v *View)) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()

	fn(&View{docs: ix.docs})
}

// Put analyses content and stores it, replacing whatever was there.
func (ix *Index) Put(uri protocol.DocumentUri, version protocol.Integer, content []byte) *Document {
	doc := Analyze(uri, version, content)

	ix.mu.Lock()
	defer ix.mu.Unlock()

	if previous, ok := ix.docs[uri]; ok {
		previous.Close()
	}

	ix.docs[uri] = doc

	return doc
}

func (ix *Index) Delete(uri protocol.DocumentUri) {
	ix.mu.Lock()
	defer ix.mu.Unlock()

	if doc, ok := ix.docs[uri]; ok {
		doc.Close()
		delete(ix.docs, uri)
	}
}

// Analyze parses and extracts a single document.
func Analyze(uri protocol.DocumentUri, version protocol.Integer, content []byte) *Document {
	path := PathFromURI(uri)

	doc := &Document{
		URI:     uri,
		Path:    path,
		Kind:    KindOf(path),
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

// ---------------------------------------------------------------- queries

func (v *View) Get(uri protocol.DocumentUri) *Document { return v.docs[uri] }

func (v *View) GetByPath(path string) *Document {
	for _, doc := range v.docs {
		if doc.Path != "" && sameFile(doc.Path, path) {
			return doc
		}
	}

	return nil
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

// TypeDecls returns every declaration of a type name, the original and each
// `extend type`, in workspace order.
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

// RelationDecls returns every `define` of relation on typeName. More than one
// means the model declares it twice, which is an error the validator reports.
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

// RelationsOfType lists the relations a type has, across all its
// declarations, in declaration order and deduplicated by name.
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

// ModuleSetFor returns the fga.mod that lists path, and the model files it
// names. A model that no fga.mod claims stands alone, and the returned mod is
// nil.
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

// Contents returns the text of path, preferring the synced buffer over disk
// so that diagnostics reflect what is on screen.
func (v *View) Contents(path string) ([]byte, error) {
	if doc := v.GetByPath(path); doc != nil {
		return doc.Content, nil
	}

	return os.ReadFile(path) //nolint:gosec
}

func sameFile(a, b string) bool {
	if a == b {
		return true
	}

	cleanA, errA := filepath.Abs(filepath.Clean(a))
	cleanB, errB := filepath.Abs(filepath.Clean(b))

	if errA != nil || errB != nil {
		return false
	}

	return cleanA == cleanB
}
