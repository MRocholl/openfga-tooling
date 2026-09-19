package tree_sitter_fga

// #cgo CFLAGS: -std=c11 -fPIC
// #include "../../src/parser.c"
import "C"

import "unsafe"

// Language returns the tree-sitter Language for the OpenFGA DSL.
func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_fga())
}
