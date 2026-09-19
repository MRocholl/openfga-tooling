package agent

import (
	"fmt"
	"sort"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/analysis"
)

// Check validates the workspace.
func (w *Workspace) Check(path string) string {
	return RenderCheck(w.CheckWorkspace(path), FormatText)
}

// References finds every use of a name.
func (w *Workspace) References(name string) string {
	return RenderReferences(w.FindReferences(name, 0), FormatText)
}

// Search matches names across the workspace.
func (w *Workspace) Search(query string) string {
	return RenderSearch(w.FindSymbols(query, maxPerFile*maxFiles), FormatText)
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
