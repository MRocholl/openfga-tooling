# Editor integration

`nvim/` is laid out as a Neovim plugin, so `parser/fga.so` and `queries/**`
are found on the runtimepath without any code. Setup only registers filetypes
and starts the server. Both artefacts are built by the root Makefile, not
committed.

## Filetypes

`*.fga` gets its own filetype. `*.fga.yaml` and `fga.mod` **keep the `yaml`
filetype** rather than taking a compound one: a dedicated filetype would
attach the server cleanly but cut those files off from every YAML plugin, to
gain nothing. The server is attached by filename instead, and `vim.lsp.start`
reuses the running client, so a model and its tests share one server and
therefore one view of the workspace.

The plugin cannot be lazy-loaded on filetype: detection has to be registered
before the first `.fga` file is opened, which may be the file nvim started on.

## The injection query

Two things about `queries/yaml/injections.scm` are easy to get wrong and fail
silently.

**`; extends` has to be the first line.** Neovim stops scanning for modelines
at the first line that is not a comment, so a modeline placed under a prose
block is never seen — and the file then *replaces* the yaml injections rather
than adding to them, taking the bash and comment injections with it.

**The offset moves a column, not a row.** `block_scalar` includes the `|`
indicator. `#offset!` keeps the original start column, which on the following
line is already past the end of a short one, so a row offset begins the region
at `schema` rather than at `model`. Starting one column past the `|` leaves a
newline and the block's indentation at the head of the region, and the grammar
treats both as whitespace.

`:InspectTree` on a `.fga.yaml` shows whether this is working: there should be
an `fga` tree nested in the `yaml` one, starting at the block's first content
line.
