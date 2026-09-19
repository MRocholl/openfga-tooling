package analysis

import (
	"path/filepath"
	"strings"
	"sync"

	protocol "github.com/tliron/glsp/protocol_3_16"
	ts "github.com/tree-sitter/go-tree-sitter"
	tsfga "github.com/mrocholl/tree-sitter-fga/bindings/go"
)

// Kind tells the three file shapes apart. They share a workspace and refer to
// one another, so the index carries all of them.
type Kind int

const (
	KindUnknown Kind = iota
	KindModel        // *.fga
	KindStoreTest    // *.fga.yaml
	KindModFile      // fga.mod
)

func KindOf(path string) Kind {
	base := filepath.Base(path)

	switch {
	case base == "fga.mod":
		return KindModFile
	case strings.HasSuffix(base, ".fga.yaml") || strings.HasSuffix(base, ".fga.yml"):
		return KindStoreTest
	case strings.HasSuffix(base, ".fga"):
		return KindModel
	default:
		return KindUnknown
	}
}

// Document is one file as the server currently understands it: the text that
// was last synced, its parse tree, and whatever symbols were extracted.
type Document struct {
	URI     protocol.DocumentUri
	Path    string
	Kind    Kind
	Version protocol.Integer
	Content []byte
	Lines   *LineIndex

	// Model documents only.
	Tree   *ts.Tree
	Module string
	Types  []*TypeDecl
	Conds  []*ConditionDecl

	// Store-test documents only.
	Store *StoreTest

	// fga.mod documents only.
	Mod *ModFile
}

func (d *Document) Close() {
	if d.Tree != nil {
		d.Tree.Close()
		d.Tree = nil
	}
}

// Root returns the tree's root node, or nil for a non-model document.
func (d *Document) Root() *ts.Node {
	if d.Tree == nil {
		return nil
	}

	return d.Tree.RootNode()
}

// Text returns the source text a node covers.
func (d *Document) Text(n *ts.Node) string {
	if n == nil {
		return ""
	}

	start, end := n.ByteRange()
	if end > uint(len(d.Content)) {
		return ""
	}

	return string(d.Content[start:end])
}

// parserPool hands out tree-sitter parsers. Parsing happens on the request
// goroutine, and a parser is not safe to share across them.
var parserPool = sync.Pool{
	New: func() any {
		parser := ts.NewParser()

		if err := parser.SetLanguage(Language()); err != nil {
			panic(err)
		}

		return parser
	},
}

var (
	languageOnce sync.Once
	language     *ts.Language
)

// Language returns the FGA tree-sitter language, loaded once.
func Language() *ts.Language {
	languageOnce.Do(func() {
		language = ts.NewLanguage(tsfga.Language())
	})

	return language
}

// Parse builds a tree for content, reusing old for incremental reparsing when
// the caller has one.
func Parse(content []byte, old *ts.Tree) *ts.Tree {
	parser, _ := parserPool.Get().(*ts.Parser)
	defer parserPool.Put(parser)

	return parser.Parse(content, old)
}
