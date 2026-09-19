package agent

// Format selects how a result is rendered.
type Format string

const (
	// FormatText is laid out for a person: grouped, with excerpts.
	FormatText Format = "text"
	// FormatAgent is one finding per line, `path:line:col: kind: text`, the
	// shape a compiler emits and every tool already knows how to slice.
	FormatAgent Format = "agent"
	// FormatJSON is the same data structured.
	FormatJSON Format = "json"
)

func ParseFormat(s string) (Format, bool) {
	switch Format(s) {
	case FormatText, FormatAgent, FormatJSON:
		return Format(s), true
	}

	return "", false
}

// Location is a place in the workspace, one-based and workspace-relative so
// it can be pasted into an editor or quoted in a reply unchanged.
type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	// EndColumn bounds the name itself, for a caret.
	EndColumn int `json:"end_column,omitempty"`
	// Text is the source line, trimmed.
	Text string `json:"text,omitempty"`
}

// Problem is one diagnostic.
type Problem struct {
	Location
	Severity string `json:"severity"`
	Message  string `json:"message"`
	// Excerpt is the offending lines with a caret, for the text form.
	Excerpt string `json:"-"`
}

// CheckResult is the outcome of validating a workspace.
type CheckResult struct {
	Problems []Problem `json:"problems"`
	Models   int       `json:"models"`
	Stores   int       `json:"stores"`
}

func (r CheckResult) Clean() bool { return len(r.Problems) == 0 }

// RefKind says what a reference is, which decides whether an agent may edit
// it and what editing it would mean.
const (
	RefKindDefinition = "definition"
	RefKindUse        = "use"
	RefKindTest       = "test"
)

// Reference is one mention of a name.
type Reference struct {
	Location
	Kind string `json:"kind"`
}

// ReferenceResult is every mention of a name, with the shape of the answer
// kept alongside it.
type ReferenceResult struct {
	Name       string       `json:"name"`
	References []Reference  `json:"references"`
	ByFile     []FileCount  `json:"by_file"`
	Total      int          `json:"total"`
	Omitted    int          `json:"omitted"`
	Note       string       `json:"note,omitempty"`
	Candidates []string     `json:"candidates,omitempty"`
	Defs       []Definition `json:"-"`
}

type FileCount struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

// Definition is one declaration site.
type Definition struct {
	Location
	Name string `json:"name"`
	Doc  string `json:"doc,omitempty"`
	// Excerpt is the declaration and its neighbours, already numbered.
	Excerpt string `json:"excerpt,omitempty"`
}

type DefinitionResult struct {
	Name        string       `json:"name"`
	Definitions []Definition `json:"definitions"`
	Candidates  []string     `json:"candidates,omitempty"`
	Note        string       `json:"note,omitempty"`
}

// Symbol is a search hit.
type Symbol struct {
	Location
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Container string `json:"container,omitempty"`
}

type SearchResult struct {
	Query   string   `json:"query"`
	Symbols []Symbol `json:"symbols"`
	Omitted int      `json:"omitted"`
}
