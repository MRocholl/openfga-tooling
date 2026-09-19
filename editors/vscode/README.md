# OpenFGA for VS Code

Syntax highlighting for `.fga`, plus everything `fga-lsp` provides:
diagnostics that match `fga model validate`, goto-definition and references
across modules, hover, symbols, completion, rename and formatting — in models,
`.fga.yaml` store tests and `fga.mod`.

## Install

Build the server first, from the repository root:

```sh
make
```

Then, from this directory:

```sh
npm install
npx vsce package
code --install-extension fga-lsp-vscode-0.1.0.vsix
```

Point `fga.server.path` at the binary if it is not on `PATH`. A relative path
resolves against the workspace root, so `bin/fga-lsp` works when the workspace
is this repository.

## Settings

| Setting | Default | Meaning |
| --- | --- | --- |
| `fga.server.path` | `fga-lsp` | path to the binary |
| `fga.server.arguments` | `[]` | extra arguments, e.g. `["-log", "/tmp/fga-lsp.log"]` |
| `fga.trace.server` | `off` | log the LSP conversation to the output channel |

## Relationship to the official extension

OpenFGA publishes its own VS Code extension with highlighting, a DSL-to-JSON
command and validation built into the extension process. This one moves the
analysis into a language server, which is what makes the same diagnostics
available to Neovim, to other editors, and to the agent tools in this
repository. Running both will give you two sets of diagnostics.
