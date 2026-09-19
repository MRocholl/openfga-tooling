package analysis

import (
	"unicode/utf16"
	"unicode/utf8"

	protocol "github.com/tliron/glsp/protocol_3_16"
	ts "github.com/tree-sitter/go-tree-sitter"
)

// LineIndex converts between byte offsets, tree-sitter points and LSP positions. See docs/diagnostics.md.
type LineIndex struct {
	content []byte
	starts  []int
}

func NewLineIndex(content []byte) *LineIndex {
	starts := make([]int, 1, 1+len(content)/32)

	for offset, b := range content {
		if b == '\n' {
			starts = append(starts, offset+1)
		}
	}

	return &LineIndex{content: content, starts: starts}
}

func (li *LineIndex) LineCount() int { return len(li.starts) }

// LineBytes returns the content of line, newline excluded.
func (li *LineIndex) LineBytes(line int) []byte {
	if line < 0 || line >= len(li.starts) {
		return nil
	}

	start := li.starts[line]

	end := len(li.content)
	if line+1 < len(li.starts) {
		end = li.starts[line+1]
	}

	for end > start && (li.content[end-1] == '\n' || li.content[end-1] == '\r') {
		end--
	}

	return li.content[start:end]
}

func (li *LineIndex) PointToPosition(p ts.Point) protocol.Position {
	line := int(p.Row)
	if line >= len(li.starts) {
		line = max(len(li.starts)-1, 0)
	}

	lineBytes := li.LineBytes(line)

	column := int(p.Column)
	if column > len(lineBytes) {
		column = len(lineBytes)
	}

	return protocol.Position{
		Line:      protocol.UInteger(line),
		Character: protocol.UInteger(utf16Len(lineBytes[:column])),
	}
}

func (li *LineIndex) PositionToByte(pos protocol.Position) uint {
	line := int(pos.Line)
	if line >= len(li.starts) {
		return uint(len(li.content))
	}

	lineBytes := li.LineBytes(line)

	return uint(li.starts[line] + utf16OffsetToByte(lineBytes, int(pos.Character)))
}

func (li *LineIndex) PositionToPoint(pos protocol.Position) ts.Point {
	line := int(pos.Line)
	if line >= len(li.starts) {
		line = max(len(li.starts)-1, 0)
	}

	lineBytes := li.LineBytes(line)

	return ts.Point{
		Row:    uint(line),
		Column: uint(utf16OffsetToByte(lineBytes, int(pos.Character))),
	}
}

func (li *LineIndex) NodeRange(n *ts.Node) protocol.Range {
	return protocol.Range{
		Start: li.PointToPosition(n.StartPosition()),
		End:   li.PointToPosition(n.EndPosition()),
	}
}

// LineRange spans a whole line, for diagnostics that name a construct but no offset within it.
func (li *LineIndex) LineRange(line int) protocol.Range {
	return protocol.Range{
		Start: protocol.Position{Line: protocol.UInteger(line)},
		End: protocol.Position{
			Line:      protocol.UInteger(line),
			Character: protocol.UInteger(utf16Len(li.LineBytes(line))),
		},
	}
}

func utf16Len(b []byte) int {
	n := 0

	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		n += utf16.RuneLen(r)

		if r == utf8.RuneError && size == 1 {
			n = n - utf16.RuneLen(r) + 1
		}

		b = b[size:]
	}

	return n
}

func utf16OffsetToByte(line []byte, offset int) int {
	if offset <= 0 {
		return 0
	}

	units, pos := 0, 0

	for pos < len(line) {
		if units >= offset {
			return pos
		}

		r, size := utf8.DecodeRune(line[pos:])
		units += utf16.RuneLen(r)
		pos += size
	}

	return len(line)
}
