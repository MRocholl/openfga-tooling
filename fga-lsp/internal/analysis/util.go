package analysis

import (
	"bytes"

	protocol "github.com/tliron/glsp/protocol_3_16"
	ts "github.com/tree-sitter/go-tree-sitter"
)

func point(line, column int) ts.Point {
	return ts.Point{Row: uint(max(line, 0)), Column: uint(max(column, 0))}
}

func indexBytes(haystack []byte, needle string) int {
	return bytes.Index(haystack, []byte(needle))
}

// InRange reports whether pos falls inside r, treating both ends as inclusive
// so that a cursor resting just after a name still resolves it.
func InRange(r protocol.Range, pos protocol.Position) bool {
	if pos.Line < r.Start.Line || pos.Line > r.End.Line {
		return false
	}

	if pos.Line == r.Start.Line && pos.Character < r.Start.Character {
		return false
	}

	if pos.Line == r.End.Line && pos.Character > r.End.Character {
		return false
	}

	return true
}

// Point builds a tree-sitter point from a zero-based line and byte column.
func Point(line, column int) ts.Point { return point(line, column) }
