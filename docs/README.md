# Knowledge

Why this code is shaped the way it is. Rationale lives here rather than in
comments, so it can be read whole and stays in one place when the code moves.

| Document | Covers |
| --- | --- |
| [grammar.md](grammar.md) | the tree-sitter grammar, its two lexical hazards, and where it is deliberately looser than the reference parser |
| [diagnostics.md](diagnostics.md) | the three diagnostic passes, which checks are ours, and why the wording is upstream's |
| [resolution.md](resolution.md) | what bounds a name, how the index stays complete, and the lifetime rules on a parse tree |
| [store-tests.md](store-tests.md) | `.fga.yaml` handling, inline models, and how ranges are found in YAML |
| [editor-integration.md](editor-integration.md) | the Neovim plugin layout, filetype choices, and the injection query |
| [versioning.md](versioning.md) | why the dependency pins track the `fga` CLI, and what to do when the corpus disagrees |
