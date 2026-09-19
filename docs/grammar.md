# Grammar

`tree-sitter-fga` mirrors the reference ANTLR grammar in
[openfga/language](https://github.com/openfga/language), schema 1.1 and 1.2.

## Two lexical hazards

The reference grammar sidesteps both with an explicit `NEWLINE` token and a
lexer mode stack. tree-sitter has neither.

**`#` is both a comment opener and a userset separator.** In `[role#assignee]`
a comment rule would swallow the rest of the line. The restriction's `#`
carries a lexical precedence, which tree-sitter resolves ahead of
longest-match, so the shorter token wins in the one parser state where both
are valid. A comment sitting inside a bracketed restriction list is the price,
and it does not occur in practice.

**Names differ between the DSL and CEL.** A type or relation name admits `.`,
`/` and `-`; inside a condition body `a.b` is member access, not one name.
They are lexed as two tokens and aliased to `identifier`, so queries and the
server see a single node type.

## Deliberate leniency

Newlines are not significant, type definitions and conditions may interleave,
and `or` / `and` / `but not` chains are not checked for homogeneity. A
half-typed buffer still yields a usable tree, which is what an editor needs.

The consequence is load-bearing: **a file can parse here and be rejected by
`fga`.** The server therefore runs the reference ANTLR parser per module file
after the tree-sitter pass, rather than trusting a clean tree. Without that,
a syntax error surfaces much later, from the modular transformer, carrying no
file attribution — see [diagnostics.md](diagnostics.md).

## Queries

`tree-sitter-fga/queries/*.scm` follows the tree-sitter CLI layout, which is
what `tree-sitter test` reads. Neovim wants `queries/<lang>/<name>.scm`. The
Neovim layout is generated into `nvim/` by the root Makefile so the two cannot
drift; nothing is duplicated in version control.
