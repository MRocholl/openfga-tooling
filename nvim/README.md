# Neovim wiring

This directory is laid out as a Neovim plugin: `parser/fga.so` and
`queries/**` are found on the runtimepath without any code, so `setup()` only
has to register filetypes and start the server.

Both artefacts are built, not committed. From the repository root:

```sh
make
```

## Install

With [lazy.nvim](https://github.com/folke/lazy.nvim):

```lua
{
  dir = vim.fn.expand("~/openfga/nvim"),
  name = "fga",
  -- Not lazy: filetype detection has to be in place before the first .fga
  -- file is opened, which may be the file nvim was started on.
  lazy = false,
  config = function()
    require("fga").setup()
  end,
}
```

`setup()` takes an optional table:

| Key | Default | Meaning |
| --- | --- | --- |
| `bin` | `<repo>/bin/fga-lsp` | path to the server binary |
| `cmd` | `{ bin }` | full command, if the server needs arguments |
| `settings` | `{}` | passed through to the server |

## What you get

`*.fga` gets the filetype `fga`, the tree-sitter parser, and the server.

`*.fga.yaml` and `fga.mod` keep the filetype `yaml`, so every YAML plugin still
works on them; the server is attached by filename instead. Inside a store
test, a `model: |` block is highlighted as FGA through an injection query, and
`vim.lsp.start` reuses the running client, so a model and its tests share one
server and therefore one view of the workspace.

## Checking it works

```vim
:lua =vim.bo.filetype
:checkhealth vim.lsp
:InspectTree
```

`:InspectTree` on a `.fga.yaml` should show a `fga` tree nested inside the
`yaml` one, starting at the block's first content line.
