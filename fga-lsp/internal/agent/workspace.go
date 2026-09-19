// Package agent exposes the model over a small, name-addressed API.
//
// The language server answers questions about a position; an agent almost
// never has one. It has a name it read in a diagnostic or a review comment --
// `document#can_view` -- and wants to know where that is defined, what uses
// it, and whether the model is currently valid. See docs/agent-api.md.
package agent

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/analysis"
)

// Workspace is a scanned directory of models and store tests.
type Workspace struct {
	root  string
	index *analysis.Index
}

func Open(root string) (*Workspace, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolving %s: %w", root, err)
	}

	index := analysis.NewIndex()
	analysis.Scan(index, abs)

	return &Workspace{root: abs, index: index}, nil
}

// Reload re-reads the workspace from disk.
func (w *Workspace) Reload() { analysis.Scan(w.index, w.root) }

func (w *Workspace) Root() string { return w.root }

// Overview is the entry point for an agent that knows nothing about the
// model: which modules exist, which types they declare, how big each is.
func (w *Workspace) Overview() string {
	var b strings.Builder

	w.index.Read(func(v *analysis.View) {
		models := v.ModelDocs()
		if len(models) == 0 {
			b.WriteString("No .fga models found under " + w.root + "\n")

			return
		}

		mods := v.ModFiles()
		stores := 0

		for _, doc := range v.Documents() {
			if doc.Kind == analysis.KindStoreTest {
				stores++
			}
		}

		fmt.Fprintf(&b, "%d %s, %d store %s", len(models), plural(len(models), "module", "modules"),
			stores, plural(stores, "test", "tests"))

		if len(mods) > 0 {
			fmt.Fprintf(&b, ", %d fga.mod", len(mods))
		}

		b.WriteString("\n\n")

		for _, doc := range models {
			label := rel(w.root, doc.Path)
			if doc.Module != "" {
				label += "  (module " + doc.Module + ")"
			}

			b.WriteString(label + "\n")

			for _, decl := range doc.Types {
				verb := "type"
				if decl.Extend {
					verb = "extend"
				}

				fmt.Fprintf(&b, "  %-7s %-28s %d %s\n",
					verb, decl.Name, len(decl.Relations),
					plural(len(decl.Relations), "relation", "relations"))
			}

			for _, cond := range doc.Conds {
				fmt.Fprintf(&b, "  %-7s %s\n", "cond", cond.Name)
			}
		}
	})

	return b.String()
}

// Describe reports what a type is: its relations, resolved across every
// module that declares or extends it.
func (w *Workspace) Describe(name string) string {
	typeName, _ := splitName(name)

	var b strings.Builder

	w.index.Read(func(v *analysis.View) {
		scope := w.scope(v)

		decls := scope.TypeDecls(typeName)
		if len(decls) == 0 {
			b.WriteString(w.notFound(scope, typeName))

			return
		}

		sites := make([]string, 0, len(decls))
		for _, decl := range decls {
			label := rel(w.root, analysis.PathFromURI(decl.URI))
			if decl.Extend {
				label += " (extend)"
			}

			sites = append(sites, label)
		}

		fmt.Fprintf(&b, "type %s  --  %s\n\n", typeName, strings.Join(sites, ", "))

		for _, decl := range decls {
			if decl.Doc != "" {
				b.WriteString(indentDoc(decl.Doc) + "\n")

				break
			}
		}

		relations := scope.RelationsOf(typeName)
		for _, relation := range relations {
			fmt.Fprintf(&b, "  %s: %s\n", relation.Name, relation.Expr)
		}

		fmt.Fprintf(&b, "\n%d %s\n", len(relations), plural(len(relations), "relation", "relations"))
	})

	return b.String()
}

// Definition locates where a name is defined.
func (w *Workspace) Definition(name string) string {
	return RenderDefinition(w.FindDefinition(name), FormatText)
}

// relationsNamed answers a bare relation name by listing the types that
// define one with that name.
func (w *Workspace) relationsNamed(scope analysis.Scope, name string) string {
	var owners []string

	for _, typeName := range scope.TypeNames() {
		if scope.HasRelation(typeName, name) {
			owners = append(owners, qualify(typeName, name))
		}
	}

	if len(owners) == 0 {
		return ""
	}

	return fmt.Sprintf("`%s` is a relation on %d %s: %s\n\nAsk again with one of these names.\n",
		name, len(owners), plural(len(owners), "type", "types"), strings.Join(owners, ", "))
}

func (w *Workspace) scope(v *analysis.View) analysis.Scope {
	for _, doc := range v.ModelDocs() {
		return analysis.ScopeFor(v, doc)
	}

	return analysis.Scope{}
}

// notFound offers the nearest names rather than a bare miss.
func (w *Workspace) notFound(scope analysis.Scope, name string) string {
	typeName, _ := splitName(name)

	candidates := scope.TypeNames()
	sort.Strings(candidates)

	near := nearest(typeName, candidates, 5)
	if len(near) == 0 {
		return fmt.Sprintf("`%s` is not in this model.\n", name)
	}

	return fmt.Sprintf("`%s` is not in this model. Nearest: %s\n", name, strings.Join(near, ", "))
}

func indentDoc(doc string) string {
	var b strings.Builder

	for _, line := range strings.Split(doc, "\n") {
		b.WriteString("  # " + line + "\n")
	}

	return b.String()
}
