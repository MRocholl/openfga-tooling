package agent

import (
	"fmt"
	"path/filepath"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/MRocholl/openfga-tooling/fga-lsp/internal/analysis"
)

const (
	contextLines  = 1
	maxPerFile    = 8
	maxFiles      = 12
	maxExcerptLen = 160
)

func rel(root, path string) string {
	if r, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(r, "..") {
		return r
	}

	return path
}

// at renders a one-based file:line:column, which is how an editor and a human
// both count and what an agent will quote back.
func at(root, path string, rng protocol.Range) string {
	return fmt.Sprintf("%s:%d:%d", rel(root, path), rng.Start.Line+1, rng.Start.Character+1)
}

// excerpt renders the lines around rng with one-based numbers, and underlines
// the range itself when it fits on a single line.
func excerpt(doc *analysis.Document, rng protocol.Range, caret bool) string {
	if doc == nil {
		return ""
	}

	first := max(int(rng.Start.Line)-contextLines, 0)
	last := min(int(rng.End.Line)+contextLines, doc.Lines.LineCount()-1)

	width := len(fmt.Sprint(last + 1))

	var b strings.Builder

	for line := first; line <= last; line++ {
		text := strings.TrimRight(string(doc.Lines.LineBytes(line)), " \t")
		if len(text) > maxExcerptLen {
			text = text[:maxExcerptLen] + "..."
		}

		fmt.Fprintf(&b, "  %*d | %s\n", width, line+1, text)

		if !caret || line != int(rng.Start.Line) || rng.Start.Line != rng.End.Line {
			continue
		}

		span := max(int(rng.End.Character-rng.Start.Character), 1)

		fmt.Fprintf(&b, "  %*s | %s%s\n",
			width, "", strings.Repeat(" ", int(rng.Start.Character)), strings.Repeat("^", span))
	}

	return b.String()
}

// qualify names a relation the way the model does, so a name read out of one
// answer can be pasted into the next query.
func qualify(typeName, relation string) string {
	if relation == "" {
		return typeName
	}

	return typeName + "#" + relation
}

// splitName parses `type`, `type#relation` or `relation`.
func splitName(name string) (typeName, relation string) {
	if hash := strings.Index(name, "#"); hash >= 0 {
		return name[:hash], name[hash+1:]
	}

	return name, ""
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}

	return many
}
