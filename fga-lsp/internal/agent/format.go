package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RenderCheck lays out a validation result.
func RenderCheck(r CheckResult, format Format) string {
	switch format {
	case FormatJSON:
		return toJSON(r)

	case FormatAgent:
		if r.Clean() {
			return fmt.Sprintf("ok: no problems in %d models, %d store tests\n", r.Models, r.Stores)
		}

		var b strings.Builder

		for _, p := range r.Problems {
			fmt.Fprintf(&b, "%s:%d:%d: %s: %s\n", p.Path, p.Line, p.Column, p.Severity, p.Message)
		}

		fmt.Fprintf(&b, "%d problems\n", len(r.Problems))

		return b.String()

	case FormatText:
	}

	if r.Clean() {
		return fmt.Sprintf("No problems. %d %s, %d store %s.\n",
			r.Models, plural(r.Models, "model", "models"), r.Stores, plural(r.Stores, "test", "tests"))
	}

	var b strings.Builder

	fmt.Fprintf(&b, "%d %s\n", len(r.Problems), plural(len(r.Problems), "problem", "problems"))

	for _, p := range r.Problems {
		fmt.Fprintf(&b, "\n%s:%d:%d %s\n%s\n", p.Path, p.Line, p.Column, p.Severity, p.Message)
		b.WriteString(p.Excerpt)
	}

	return b.String()
}

// RenderReferences lays out a reference search.
//
// The agent form is flat and complete: one `path:line:col: kind: source` per
// hit, in path order, with no grouping to unpick and nothing dropped unless a
// limit asked for it. That is what an agent is going to read, filter and cite.
func RenderReferences(r ReferenceResult, format Format) string {
	switch format {
	case FormatJSON:
		return toJSON(r)

	case FormatAgent:
		var b strings.Builder

		if r.Note != "" {
			fmt.Fprintf(&b, "note: %s\n", r.Note)
		}

		for _, c := range r.Candidates {
			fmt.Fprintf(&b, "candidate: %s\n", c)
		}

		for _, ref := range r.References {
			fmt.Fprintf(&b, "%s:%d:%d: %s: %s\n", ref.Path, ref.Line, ref.Column, ref.Kind, ref.Text)
		}

		if r.Total > 0 {
			fmt.Fprintf(&b, "%d %s in %d %s", r.Total, plural(r.Total, "reference", "references"), len(r.ByFile), plural(len(r.ByFile), "file", "files"))

			if r.Omitted > 0 {
				fmt.Fprintf(&b, ", %d omitted by --limit", r.Omitted)
			}

			b.WriteString("\n")
		} else if r.Note == "" {
			fmt.Fprintf(&b, "0 references to %s\n", r.Name)
		}

		return b.String()

	case FormatText:
	}

	return renderReferencesText(r)
}

func renderReferencesText(r ReferenceResult) string {
	var b strings.Builder

	if r.Note != "" {
		fmt.Fprintf(&b, "%s: %s\n", r.Note, strings.Join(r.Candidates, ", "))

		return b.String()
	}

	if r.Total == 0 {
		fmt.Fprintf(&b, "`%s` is not in this model.", r.Name)

		if len(r.Candidates) > 0 {
			fmt.Fprintf(&b, " Nearest: %s", strings.Join(r.Candidates, ", "))
		}

		b.WriteString("\n")

		return b.String()
	}

	fmt.Fprintf(&b, "References to `%s`: %d across %d %s\n",
		r.Name, r.Total, len(r.ByFile), plural(len(r.ByFile), "file", "files"))

	// Past a point the individual lines stop being readable and the shape of
	// the answer is the answer.
	if r.Total > maxPerFile*maxFiles {
		b.WriteString("\n")

		for _, f := range r.ByFile {
			fmt.Fprintf(&b, "  %-40s %d\n", f.Path, f.Count)
		}

		b.WriteString("\nAsk for one file to see the lines.\n")

		return b.String()
	}

	byPath := map[string][]Reference{}
	for _, ref := range r.References {
		byPath[ref.Path] = append(byPath[ref.Path], ref)
	}

	for i, f := range r.ByFile {
		if i >= maxFiles {
			fmt.Fprintf(&b, "\n  ... and %d more files\n", len(r.ByFile)-maxFiles)

			break
		}

		hits := byPath[f.Path]

		fmt.Fprintf(&b, "\n%s (%d)\n", f.Path, len(hits))

		shown := min(len(hits), maxPerFile)
		for _, ref := range hits[:shown] {
			fmt.Fprintf(&b, "  %d | %s\n", ref.Line, ref.Text)
		}

		if len(hits) > shown {
			fmt.Fprintf(&b, "  ... %d more\n", len(hits)-shown)
		}
	}

	if r.Omitted > 0 {
		fmt.Fprintf(&b, "\n%d omitted by --limit\n", r.Omitted)
	}

	return b.String()
}

// RenderDefinition lays out a definition lookup.
func RenderDefinition(r DefinitionResult, format Format) string {
	switch format {
	case FormatJSON:
		return toJSON(r)

	case FormatAgent:
		var b strings.Builder

		if r.Note != "" {
			fmt.Fprintf(&b, "note: %s\n", r.Note)
		}

		for _, c := range r.Candidates {
			fmt.Fprintf(&b, "candidate: %s\n", c)
		}

		for _, d := range r.Definitions {
			fmt.Fprintf(&b, "%s:%d:%d: definition: %s\n", d.Path, d.Line, d.Column, d.Text)
		}

		if len(r.Definitions) == 0 && r.Note == "" {
			fmt.Fprintf(&b, "0 definitions of %s\n", r.Name)
		}

		return b.String()

	case FormatText:
	}

	var b strings.Builder

	if r.Note != "" {
		fmt.Fprintf(&b, "%s: %s\n", r.Note, strings.Join(r.Candidates, ", "))

		return b.String()
	}

	if len(r.Definitions) == 0 {
		fmt.Fprintf(&b, "`%s` is not in this model.", r.Name)

		if len(r.Candidates) > 0 {
			fmt.Fprintf(&b, " Nearest: %s", strings.Join(r.Candidates, ", "))
		}

		b.WriteString("\n")

		return b.String()
	}

	fmt.Fprintf(&b, "Definition of `%s`:\n", r.Name)

	for _, d := range r.Definitions {
		fmt.Fprintf(&b, "\n%s:%d:%d\n", d.Path, d.Line, d.Column)

		if d.Doc != "" {
			b.WriteString(indentDoc(d.Doc))
		}

		b.WriteString(d.Excerpt)
	}

	return b.String()
}

// RenderSearch lays out a symbol search.
func RenderSearch(r SearchResult, format Format) string {
	switch format {
	case FormatJSON:
		return toJSON(r)

	case FormatAgent:
		var b strings.Builder

		for _, s := range r.Symbols {
			fmt.Fprintf(&b, "%s:%d:%d: %s: %s\n", s.Path, s.Line, s.Column, s.Kind, qualify(s.Container, s.Name))
		}

		fmt.Fprintf(&b, "%d %s", len(r.Symbols), plural(len(r.Symbols), "match", "matches"))

		if r.Omitted > 0 {
			fmt.Fprintf(&b, ", %d omitted by --limit", r.Omitted)
		}

		b.WriteString("\n")

		return b.String()

	case FormatText:
	}

	if len(r.Symbols) == 0 {
		return fmt.Sprintf("Nothing matches %q.\n", r.Query)
	}

	var b strings.Builder

	fmt.Fprintf(&b, "%d %s for %q\n\n",
		len(r.Symbols), plural(len(r.Symbols), "match", "matches"), r.Query)

	for _, s := range r.Symbols {
		fmt.Fprintf(&b, "  %-44s %s:%d:%d\n", qualify(s.Container, s.Name), s.Path, s.Line, s.Column)
	}

	if r.Omitted > 0 {
		fmt.Fprintf(&b, "  ... %d more\n", r.Omitted)
	}

	return b.String()
}

func toJSON(v any) string {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("{\"error\": %q}\n", err.Error())
	}

	return string(out) + "\n"
}
