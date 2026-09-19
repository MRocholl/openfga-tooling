# tree-sitter-fga

A tree-sitter grammar for the OpenFGA authorization model DSL, schema 1.1 and
1.2, mirroring the reference ANTLR grammar in
[openfga/language](https://github.com/openfga/language).

Covers modular models (`module`, `extend type`), type restrictions with
usersets, wildcards and conditions, and CEL condition bodies.

```sh
tree-sitter generate && tree-sitter test
```

## Two lexical hazards

The reference grammar sidesteps both with an explicit `NEWLINE` token and a
lexer mode stack, neither of which tree-sitter has.

**`#` is both a comment and a separator.** In `[role#assignee]` a comment rule
would swallow the rest of the line. The restriction's `#` carries a lexical
precedence, which tree-sitter resolves *before* longest-match, so the shorter
token wins in the one state where both are valid.

**Names differ between the DSL and CEL.** A type or relation name admits `.`,
`/` and `-`; inside a condition body `a.b` is member access. The two are lexed
as separate tokens and aliased to `identifier`, so queries see one node type.

## Deviations, all toward leniency

Newlines are not significant, type definitions and conditions may interleave,
and `or` / `and` / `but not` chains are not checked for homogeneity. A
half-typed buffer still yields a usable tree, which is the point.

This means a file can parse here and be rejected by `fga`. That is handled in
the server, which runs the reference parser per module file after this one —
see `antlrSyntaxErrors` in `fga-lsp/internal/diag`.

## Queries

`queries/*.scm` follow the tree-sitter CLI layout. The Neovim layout
(`queries/fga/*.scm`) is generated into `../nvim` by the root Makefile, so the
two cannot drift.

`editors/nvim/queries/yaml/injections.scm` injects the DSL into a store test's
`model: |` block.
