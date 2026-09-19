# Upstream conformance corpus

Test data vendored from [openfga/language](https://github.com/openfga/language),
Apache-2.0, at commit `0d2ad7fb7c40`.

This is the same corpus the reference Go, JS and Java implementations are
tested against. Both halves of this project are checked against it: the
grammar has to parse every model in it, and the server has to agree with it
about which models are valid and where exactly they are wrong.

The commit is not arbitrary. `fga` v0.8.0 -- the CLI this repository's
diagnostics are meant to agree with -- pins
`github.com/openfga/language/pkg/go` at exactly this commit
(`v0.3.2-0.20260818192608-0d2ad7fb7c40`), alongside
`github.com/openfga/openfga v1.20.0`. `fga-lsp` pins the same two, so a model
this server accepts is one `fga model validate` accepts. Bump all three
together, re-run this suite, and fix whatever moves.

| Path | Upstream path | What it holds |
| --- | --- | --- |
| `dsl-syntax-validation-cases.yaml` | `tests/data/` | DSL snippets with their expected syntax errors and positions |
| `dsl-semantic-validation-cases.yaml` | `tests/data/` | syntactically valid models with their expected semantic errors |
| `fga-mod-transformer-cases.yaml` | `tests/data/` | `fga.mod` files with their expected errors |
| `models/*.fga` | `tests/data/transformer/*/authorization-model.fga` | valid models, from a bare schema to GitHub and Google Drive |
| `transformer-module/` | `tests/data/transformer-module/` | modular models, including one with deliberate errors |
| `stores/` | `tests/data/stores/` | `.fga.yaml` store tests |

Positions in the case files are zero-based.
