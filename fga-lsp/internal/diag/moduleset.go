package diag

import (
	"errors"
	"regexp"
	"strconv"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/openfga/language/pkg/go/transformer"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/mrocholl/fga-lsp/internal/analysis"
)

// moduleSetInfo is the unit the reference transformer works on: either the
// files an fga.mod lists, or a single self-contained model.
type moduleSetInfo struct {
	mod     *analysis.Document
	primary *analysis.Document
	docs    []*analysis.Document
}

type problem struct {
	uri     protocol.DocumentUri
	rng     protocol.Range
	message string
}

func moduleSet(view *analysis.View, doc *analysis.Document) moduleSetInfo {
	set := moduleSetInfo{
		primary: doc,
		docs:    []*analysis.Document{doc},
	}

	mod, members := view.ModuleSetFor(doc.Path)
	if mod == nil {
		return set
	}

	set.mod = mod
	set.docs = nil

	for _, path := range members {
		if member := view.GetByPath(path); member != nil {
			set.docs = append(set.docs, member)
		}
	}

	if len(set.docs) == 0 {
		set.docs = []*analysis.Document{doc}
	}

	return set
}

func (s moduleSetInfo) contains(uri protocol.DocumentUri) bool {
	for _, doc := range s.docs {
		if doc.URI == uri {
			return true
		}
	}

	return false
}

// fallback is where a diagnostic lands when nothing in the set claims it.
func (s moduleSetInfo) fallback() (protocol.DocumentUri, protocol.Range) {
	target := s.primary
	if s.mod != nil {
		target = s.mod
	}

	return target.URI, target.Lines.LineRange(0)
}

// build assembles the set into an authorization model, returning the
// transformation failures located in their own files.
func (s moduleSetInfo) build(view *analysis.View) (*openfgav1.AuthorizationModel, []problem) {
	if s.mod != nil {
		return s.buildModular(view)
	}

	// A module file outside any fga.mod cannot stand on its own: it has no
	// schema version and its types may be extensions of another module's.
	// There is nothing to report that the user did not already know.
	if s.primary.Module != "" {
		return nil, nil
	}

	model, err := transformer.TransformDSLToProto(string(s.primary.Content))
	if err == nil {
		return model, nil
	}

	var problems []problem

	for _, single := range flatten(err) {
		line, column, message := parseSyntaxError(single.Error())

		problems = append(problems, problem{
			uri:     s.primary.URI,
			rng:     caretRange(s.primary, line, column),
			message: message,
		})
	}

	return nil, problems
}

func (s moduleSetInfo) buildModular(view *analysis.View) (*openfgav1.AuthorizationModel, []problem) {
	var (
		files    []transformer.ModuleFile
		problems []problem
	)

	for _, entry := range s.mod.Mod.Contents {
		contents, err := view.Contents(entry.Path)
		if err != nil {
			problems = append(problems, problem{
				uri:     s.mod.URI,
				rng:     entry.Range,
				message: "cannot read " + entry.Value + ": " + err.Error(),
			})

			continue
		}

		files = append(files, transformer.ModuleFile{Name: entry.Value, Contents: string(contents)})
	}

	if len(problems) > 0 {
		return nil, problems
	}

	model, err := transformer.TransformModuleFilesToModel(files, s.mod.Mod.Schema)
	if err == nil {
		return model, nil
	}

	for _, single := range flatten(err) {
		problems = append(problems, s.locate(view, single))
	}

	return nil, problems
}

// locate places a module transformation error in the file it names.
func (s moduleSetInfo) locate(view *analysis.View, err error) problem {
	var single *transformer.ModuleTransformationSingleError
	if !errors.As(err, &single) {
		uri, rng := s.fallback()

		return problem{uri: uri, rng: rng, message: err.Error()}
	}

	for _, entry := range s.mod.Mod.Contents {
		if entry.Value != single.File {
			continue
		}

		doc := view.GetByPath(entry.Path)
		if doc == nil {
			break
		}

		return problem{
			uri:     doc.URI,
			rng:     spanRange(doc, single.Line.Start, single.Column.Start, single.Line.End, single.Column.End),
			message: single.Msg,
		}
	}

	uri, rng := s.fallback()

	return problem{uri: uri, rng: rng, message: single.Error()}
}

var syntaxErrorPattern = regexp.MustCompile(`^syntax error at line=(-?\d+), column=(-?\d+): (.*)$`)

// parseSyntaxError recovers the position the transformer keeps unexported.
// Both numbers are already zero-based. A message in any other shape is passed
// through whole and pinned to the first line.
func parseSyntaxError(text string) (line, column int, message string) {
	match := syntaxErrorPattern.FindStringSubmatch(text)
	if match == nil {
		return 0, 0, text
	}

	line, _ = strconv.Atoi(match[1])
	column, _ = strconv.Atoi(match[2])

	return line, column, match[3]
}

// caretRange underlines the word at line/column, or the whole line when there
// is no word there.
func caretRange(doc *analysis.Document, line, column int) protocol.Range {
	if line < 0 || line >= doc.Lines.LineCount() {
		return doc.Lines.LineRange(0)
	}

	content := doc.Lines.LineBytes(line)
	if column < 0 || column >= len(content) {
		return doc.Lines.LineRange(line)
	}

	end := column
	for end < len(content) && !isSpace(content[end]) {
		end++
	}

	if end == column {
		end = min(column+1, len(content))
	}

	return protocol.Range{
		Start: doc.Lines.PointToPosition(analysis.Point(line, column)),
		End:   doc.Lines.PointToPosition(analysis.Point(line, end)),
	}
}

func spanRange(doc *analysis.Document, startLine, startColumn, endLine, endColumn int) protocol.Range {
	if startLine < 0 || startLine >= doc.Lines.LineCount() {
		return doc.Lines.LineRange(0)
	}

	if endLine < startLine || endLine >= doc.Lines.LineCount() || endColumn <= startColumn {
		return caretRange(doc, startLine, startColumn)
	}

	return protocol.Range{
		Start: doc.Lines.PointToPosition(analysis.Point(startLine, startColumn)),
		End:   doc.Lines.PointToPosition(analysis.Point(endLine, endColumn)),
	}
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

// scoped returns the documents that bound name resolution: the module set
// when an fga.mod defines one, nil to mean "fall back to the workspace".
func (s moduleSetInfo) scoped() []*analysis.Document {
	if s.mod == nil {
		return nil
	}

	return s.docs
}

// antlrSyntaxErrors runs the reference parser over one module file.
//
// The tree-sitter grammar is deliberately the more permissive of the two, so
// a file can yield a clean tree and still be rejected by `fga`. Running ANTLR
// per file, rather than letting TransformModuleFilesToModel discover it,
// is what puts the error in the module that caused it: a syntax failure
// inside the modular transform arrives as a type with no File field, and
// would otherwise be reported against the fga.mod.
func antlrSyntaxErrors(doc *analysis.Document, result Result) bool {
	_, listener := transformer.ParseDSL(string(doc.Content))
	if listener == nil || listener.Errors == nil || len(listener.Errors.Errors) == 0 {
		return false
	}

	for _, single := range listener.Errors.Errors {
		line, column, message := parseSyntaxError(single.Error())

		result.add(doc.URI, diagnostic(
			caretRange(doc, line, column),
			message,
			protocol.DiagnosticSeverityError,
		))
	}

	return true
}
