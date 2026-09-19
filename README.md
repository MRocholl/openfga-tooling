# OpenFGA editor support

A tree-sitter grammar and a language server for the
[OpenFGA](https://openfga.dev) authorization model DSL, its `.fga.yaml` store
tests, and `fga.mod`.

Neither existed. Upstream ships an ANTLR grammar, a VS Code extension and a
JetBrains plugin, but the validation in those is built into the editor rather
than exposed over LSP, and the one community tree-sitter grammar predates
schema 1.2. This repository covers both gaps in a form any editor can use.

| Directory | What it is |
| --- | --- |
| [`tree-sitter-fga/`](tree-sitter-fga) | the grammar: parsing and highlighting |
| [`fga-lsp/`](fga-lsp) | the language server: diagnostics, navigation, completion, rename, formatting |
| [`nvim/`](nvim) | a Neovim plugin that wires the two together |
| [`editors/vscode/`](editors/vscode) | a VS Code extension |

Beyond editors, the same analysis is served to agents:

```sh
fga-lsp                      # LSP over stdin/stdout, for an editor
fga-lsp check -C <dir>       # a shell command, for an agent or for CI
fga-lsp mcp <dir>            # MCP, for a host without a shell
```

The agent tools are name-addressed rather than position-addressed, because an
agent has `document#can_view`, not a line and a column. See
[docs/agent-api.md](docs/agent-api.md).

## Build

```sh
make          # bin/fga-lsp, nvim/parser/fga.so, nvim/queries/**
make test     # grammar corpus, server unit tests, upstream conformance, go vet
```

Needs the `tree-sitter` CLI and Go. See [`nvim/README.md`](nvim/README.md) for
the editor side.

## Agreeing with the CLI

The server's diagnostics are meant to say exactly what `fga model validate`
says, in the same words and on the same range. That is a deliberate constraint
rather than a nicety: a model the editor accepts and CI rejects is worse than
no diagnostics at all.

It is held in place by two things. `fga-lsp` pins the same
`github.com/openfga/language/pkg/go` and `github.com/openfga/openfga` versions
that the `fga` release pins — currently v0.8.0 — and it reuses openfga's own
transformer and typesystem rather than reimplementing them. The checks the Go
package does not carry, which upstream implements only in JS and Java, are
reimplemented here against the vendored conformance corpus, message for
message.

When the corpus and the pinned server disagree, the pinned server wins and the
case is skipped with a note; see `aheadOfTheServer` in
`fga-lsp/internal/conformance`. Bump all three pins together.

## Licence

Apache-2.0, matching the upstream grammar this one is derived from.
