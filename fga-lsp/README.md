# fga-lsp

A language server for the OpenFGA DSL, `.fga.yaml` store tests and `fga.mod`.
One static binary, no runtime dependencies.

```sh
go build -o ../bin/fga-lsp .      # or: make, from the repository root
fga-lsp                            # speaks LSP over stdin/stdout
```

`-log <file>` writes logs somewhere; without it they are discarded, because
stdout carries the protocol. `-tcp <addr>` serves over TCP instead.

```sh
fga-lsp check -C <dir>   # the same analysis from a shell; exits 1 on findings
fga-lsp mcp <dir>        # the same analysis as MCP tools
fga-lsp help             # every query
```

See [../docs/agent-api.md](../docs/agent-api.md).

## What it does

**Diagnostics** in three passes, cheapest first, each gating the next: the
tree-sitter tree, which has an answer for a half-typed buffer; the reference
ANTLR parser, run per module file so a syntax error names the module rather
than the `fga.mod`; then openfga's own typesystem, the same check
`fga model validate` runs. Name resolution runs even when the DSL does not
compile, because an unknown relation is what a half-finished edit looks like,
and it underlines the word rather than the line.

**Navigation and editing**: goto-definition and find-references across
modules, hover showing the definition a name resolves to and where it lives,
document and workspace symbols, context-aware completion, workspace-wide
rename, and formatting.

**Store tests** are checked against the model they point at, whether that is a
`model_file:` or an inline `model: |` block: tuple and check syntax, and
whether every type and relation named is actually in that model, with a
nearest-name suggestion when it is not. Renaming a relation reaches its uses
in checks and assertions.

## Scope

An `fga.mod` bounds name resolution, so two unrelated models in one workspace
do not lend each other their types. A module no `fga.mod` claims falls back to
the workspace, which beats reporting every sibling's types as unknown.

Referenced files are loaded from disk to a fixed point, so a client that syncs
only the files it opens still sees whole module sets.

## Tests

```sh
go test ./...
```

`internal/conformance` runs the vendored upstream corpus — see
`testdata/upstream/README.md`. The grammar has to parse every model in it, and
the server has to agree with it on which models are valid and exactly where
they are wrong, message for message.

`TestExternalWorkspace` points the server at a real model tree and asserts it
stays quiet:

```sh
FGA_LSP_WORKSPACE=/path/to/authz go test ./internal/server/
```
