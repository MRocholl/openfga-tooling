package agent

import (
	"fmt"
	"sort"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/analysis"
	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/diag"
	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/feature"
)

// Check validates the workspace and reports what is wrong, grouped by file.
func (w *Workspace) Check(path string) string {
	var b strings.Builder

	w.index.Read(func(v *analysis.View) {
		results := diag.Result{}

		for _, doc := range v.Documents() {
			if doc.Kind == analysis.KindUnknown {
				continue
			}

			if path != "" && rel(w.root, doc.Path) != path && doc.Path != path {
				continue
			}

			for uri, diags := range diag.Compute(v, doc) {
				if len(diags) > 0 {
					results[uri] = append(results[uri], diags...)
				}
			}
		}

		total := 0
		for _, diags := range results {
			total += len(diags)
		}

		if total == 0 {
			models, stores := counts(v)
			fmt.Fprintf(&b, "No problems. %d %s, %d store %s.\n",
				models, plural(models, "model", "models"), stores, plural(stores, "test", "tests"))

			return
		}

		fmt.Fprintf(&b, "%d %s\n", total, plural(total, "problem", "problems"))

		uris := make([]protocol.DocumentUri, 0, len(results))
		for uri := range results {
			uris = append(uris, uri)
		}

		sort.Slice(uris, func(i, j int) bool { return uris[i] < uris[j] })

		for _, uri := range uris {
			doc := v.Get(uri)
			diags := dedupe(results[uri])

			shown := min(len(diags), maxPerFile)

			for _, d := range diags[:shown] {
				fmt.Fprintf(&b, "\n%s %s\n%s\n",
					at(w.root, analysis.PathFromURI(uri), d.Range), severity(d), d.Message)
				b.WriteString(excerpt(doc, d.Range, true))
			}

			if len(diags) > shown {
				fmt.Fprintf(&b, "\n  ... %d more in %s\n",
					len(diags)-shown, rel(w.root, analysis.PathFromURI(uri)))
			}
		}
	})

	return b.String()
}

// References finds every use of a name. A long answer is compressed to
// per-file counts first, so the agent can ask for one file rather than read
// through all of them.
func (w *Workspace) References(name string) string {
	typeName, relation := splitName(name)

	var b strings.Builder

	w.index.Read(func(v *analysis.View) {
		scope := w.scope(v)

		target := feature.Target{Kind: feature.TargetType, Name: typeName}
		if relation != "" {
			target = feature.Target{
				Kind:       feature.TargetRelation,
				Name:       relation,
				OwnerTypes: []string{typeName},
			}
		} else if !scope.HasType(typeName) {
			if hits := w.relationsNamed(scope, typeName); hits != "" {
				b.WriteString(hits)

				return
			}
		}

		type hit struct {
			path string
			rng  protocol.Range
			uri  protocol.DocumentUri
		}

		byFile := map[string][]hit{}
		total := 0

		for _, doc := range v.Documents() {
			for _, symbol := range feature.Symbols(v, doc) {
				if symbol.Kind != target.Kind || symbol.Name != target.Name {
					continue
				}

				if target.Kind == feature.TargetRelation && len(symbol.OwnerTypes) > 0 {
					if !contains(symbol.OwnerTypes, typeName) {
						continue
					}
				}

				path := rel(w.root, doc.Path)
				byFile[path] = append(byFile[path], hit{path: path, rng: symbol.Range, uri: doc.URI})
				total++
			}
		}

		if total == 0 {
			b.WriteString(w.notFound(scope, name))

			return
		}

		files := make([]string, 0, len(byFile))
		for path := range byFile {
			files = append(files, path)
		}

		sort.Slice(files, func(i, j int) bool {
			if len(byFile[files[i]]) != len(byFile[files[j]]) {
				return len(byFile[files[i]]) > len(byFile[files[j]])
			}

			return files[i] < files[j]
		})

		fmt.Fprintf(&b, "References to `%s`: %d across %d %s\n",
			name, total, len(files), plural(len(files), "file", "files"))

		// Past a point the excerpts stop being readable and the counts are
		// the more useful answer.
		if total > maxPerFile*maxFiles {
			b.WriteString("\n")

			for _, path := range files {
				fmt.Fprintf(&b, "  %-40s %d\n", path, len(byFile[path]))
			}

			b.WriteString("\nAsk for one file to see the lines.\n")

			return
		}

		for i, path := range files {
			if i >= maxFiles {
				fmt.Fprintf(&b, "\n  ... and %d more files\n", len(files)-maxFiles)

				break
			}

			hits := byFile[path]

			fmt.Fprintf(&b, "\n%s (%d)\n", path, len(hits))

			shown := min(len(hits), maxPerFile)
			for _, h := range hits[:shown] {
				doc := v.Get(h.uri)
				fmt.Fprintf(&b, "  %d | %s\n", h.rng.Start.Line+1,
					strings.TrimSpace(string(doc.Lines.LineBytes(int(h.rng.Start.Line)))))
			}

			if len(hits) > shown {
				fmt.Fprintf(&b, "  ... %d more\n", len(hits)-shown)
			}
		}
	})

	return b.String()
}

// Search matches type, relation and condition names across the workspace.
func (w *Workspace) Search(query string) string {
	var b strings.Builder

	w.index.Read(func(v *analysis.View) {
		symbols := feature.WorkspaceSymbols(v, query)
		if len(symbols) == 0 {
			fmt.Fprintf(&b, "Nothing matches %q.\n", query)

			return
		}

		fmt.Fprintf(&b, "%d %s for %q\n\n", len(symbols), plural(len(symbols), "match", "matches"), query)

		shown := min(len(symbols), maxPerFile*maxFiles)
		for _, symbol := range symbols[:shown] {
			owner := ""
			if symbol.ContainerName != nil && *symbol.ContainerName != "" {
				owner = *symbol.ContainerName + "#"
			}

			fmt.Fprintf(&b, "  %-44s %s\n", owner+symbol.Name,
				at(w.root, analysis.PathFromURI(protocol.DocumentUri(symbol.Location.URI)), symbol.Location.Range))
		}

		if len(symbols) > shown {
			fmt.Fprintf(&b, "  ... %d more\n", len(symbols)-shown)
		}
	})

	return b.String()
}

func counts(v *analysis.View) (models, stores int) {
	for _, doc := range v.Documents() {
		switch doc.Kind {
		case analysis.KindModel:
			models++
		case analysis.KindStoreTest:
			stores++
		case analysis.KindModFile, analysis.KindUnknown:
		}
	}

	return models, stores
}

func severity(d protocol.Diagnostic) string {
	if d.Severity != nil && *d.Severity == protocol.DiagnosticSeverityWarning {
		return "warning"
	}

	return "error"
}

func dedupe(diags []protocol.Diagnostic) []protocol.Diagnostic {
	seen := map[string]struct{}{}

	out := make([]protocol.Diagnostic, 0, len(diags))

	for _, d := range diags {
		key := fmt.Sprintf("%d:%d:%s", d.Range.Start.Line, d.Range.Start.Character, d.Message)
		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}

		out = append(out, d)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Range.Start.Line != out[j].Range.Start.Line {
			return out[i].Range.Start.Line < out[j].Range.Start.Line
		}

		return out[i].Range.Start.Character < out[j].Range.Start.Character
	})

	return out
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}

	return false
}

// nearest returns the closest names by edit distance, for a miss.
func nearest(name string, candidates []string, limit int) []string {
	type scored struct {
		name     string
		distance int
	}

	var hits []scored

	for _, candidate := range candidates {
		d := editDistance(name, candidate)
		if d <= len(name)/2+2 {
			hits = append(hits, scored{candidate, d})
		}
	}

	sort.Slice(hits, func(i, j int) bool { return hits[i].distance < hits[j].distance })

	out := make([]string, 0, limit)
	for i, hit := range hits {
		if i >= limit {
			break
		}

		out = append(out, hit.name)
	}

	return out
}

func editDistance(a, b string) int {
	if a == b {
		return 0
	}

	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)

	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {
		current[0] = i

		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			current[j] = min(previous[j]+1, min(current[j-1]+1, previous[j-1]+cost))
		}

		previous, current = current, previous
	}

	return previous[len(b)]
}
