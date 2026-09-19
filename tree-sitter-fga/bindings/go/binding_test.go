package tree_sitter_fga_test

import (
	"testing"

	tree_sitter_fga "github.com/MRocholl/openfga-tooling/tree-sitter-fga/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCanLoadGrammar(t *testing.T) {
	language := tree_sitter.NewLanguage(tree_sitter_fga.Language())
	if language == nil {
		t.Fatal("Error loading Fga grammar")
	}

	parser := tree_sitter.NewParser()
	defer parser.Close()

	if err := parser.SetLanguage(language); err != nil {
		t.Fatalf("Error setting language: %v", err)
	}

	tree := parser.Parse([]byte("module core\n\ntype user\n"), nil)
	defer tree.Close()

	if tree.RootNode().HasError() {
		t.Fatalf("unexpected parse error: %s", tree.RootNode().ToSexp())
	}
}
