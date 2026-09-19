package analysis

import (
	"errors"
	"path/filepath"

	"github.com/openfga/language/pkg/go/transformer"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

// ModEntry is one file listed under `contents:` in an fga.mod.
type ModEntry struct {
	Value string
	Path  string
	Range protocol.Range
}

// ModFile is a parsed fga.mod. Parsing is delegated to the reference
// implementation so that a file this server accepts is one `fga` accepts.
type ModFile struct {
	Schema   string
	Contents []ModEntry
	Errors   []ModError
}

type ModError struct {
	Message string
	Range   protocol.Range
}

func ParseModFile(doc *Document) *ModFile {
	dir := filepath.Dir(doc.Path)
	mod := &ModFile{}

	parsed, err := transformer.TransformModFile(string(doc.Content))
	if err != nil {
		var multi *transformer.ModFileValidationMultipleError
		if errors.As(err, &multi) {
			for _, e := range multi.Errors {
				var single *transformer.ModFileValidationError
				if errors.As(e, &single) {
					mod.Errors = append(mod.Errors, ModError{
						Message: single.Msg,
						Range:   pointRange(doc.Lines, single.Line, single.Column),
					})

					continue
				}

				mod.Errors = append(mod.Errors, ModError{Message: e.Error()})
			}
		} else {
			mod.Errors = append(mod.Errors, ModError{
				Message: err.Error(),
				Range:   doc.Lines.LineRange(0),
			})
		}
	}

	if parsed == nil {
		return mod
	}

	mod.Schema = parsed.Schema.Value

	for _, entry := range parsed.Contents.Value {
		mod.Contents = append(mod.Contents, ModEntry{
			Value: entry.Value,
			Path:  filepath.Join(dir, entry.Value),
			Range: valueRange(doc.Lines, entry.Line, entry.Value),
		})
	}

	return mod
}

// pointRange turns a zero-based line/column into a one-character range,
// widened to the whole line when the column is unknown.
func pointRange(lines *LineIndex, line, column int) protocol.Range {
	if line < 0 {
		line = 0
	}

	if column <= 0 {
		return lines.LineRange(line)
	}

	start := lines.PointToPosition(point(line, column))
	end := start
	end.Character++

	return protocol.Range{Start: start, End: end}
}

// valueRange locates value within the given line, so a diagnostic underlines
// the entry rather than the whole line.
func valueRange(lines *LineIndex, line int, value string) protocol.Range {
	return valueRangeFrom(lines, line, 0, value)
}

// valueRangeFrom is valueRange, searching from a known column so that a
// repeated substring earlier on the line is not matched instead.
func valueRangeFrom(lines *LineIndex, line, from int, value string) protocol.Range {
	if line < 0 || value == "" {
		return lines.LineRange(max(line, 0))
	}

	content := lines.LineBytes(line)

	if from < 0 || from > len(content) {
		from = 0
	}

	offset := indexBytes(content[from:], value)
	if offset < 0 {
		// The hint was wrong -- a multi-line scalar, say. Fall back to the
		// whole line rather than to a confidently wrong span.
		if offset = indexBytes(content, value); offset < 0 {
			return lines.LineRange(line)
		}
	} else {
		offset += from
	}

	return protocol.Range{
		Start: lines.PointToPosition(point(line, offset)),
		End:   lines.PointToPosition(point(line, offset+len(value))),
	}
}
