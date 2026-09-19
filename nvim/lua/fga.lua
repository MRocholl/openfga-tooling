-- Neovim wiring for the OpenFGA authorization model DSL.
--
-- This directory is laid out as a plugin, so `parser/fga.so` and
-- `queries/**` are found by Neovim on their own once it is on the
-- runtimepath. What is left for setup() is filetype detection and starting
-- the language server.
--
-- Build the two artefacts first, from the repository root:
--
--     make
--
-- Then point a plugin manager at this directory; see README.md.

local M = {}

-- plugin_root is this file's grandparent: <repo>/nvim.
local function plugin_root()
  return vim.fn.fnamemodify(debug.getinfo(1, "S").source:sub(2), ":h:h")
end

-- Store tests and fga.mod stay YAML. Giving them a filetype of their own
-- would take them out of reach of every yaml plugin, to gain nothing: the
-- server is attached by filename below, and the DSL inside a `model: |` block
-- is highlighted by the injection query rather than by the filetype.
local yaml_patterns = {
  ".*%.fga%.ya?ml",
  ".*/fga%.mod",
  "fga%.mod",
}

local function register_filetypes()
  local pattern = {}
  for _, p in ipairs(yaml_patterns) do
    pattern[p] = "yaml"
  end

  vim.filetype.add({
    extension = { fga = "fga" },
    pattern = pattern,
  })
end

-- matches_fga_file reports whether a buffer holds something the server can
-- answer for, by name rather than by filetype.
local function matches_fga_file(name)
  if name == "" then
    return false
  end

  local base = vim.fn.fnamemodify(name, ":t")

  return base:match("%.fga$") ~= nil
    or base:match("%.fga%.ya?ml$") ~= nil
    or base == "fga.mod"
end

function M.setup(opts)
  opts = opts or {}

  local root = plugin_root()
  local cmd = opts.cmd or { opts.bin or (vim.fn.fnamemodify(root, ":h") .. "/bin/fga-lsp") }

  register_filetypes()

  -- Highlighting: the parser ships next to this file, so the only thing
  -- missing is starting the tree for a filetype nvim-treesitter does not
  -- know about.
  vim.api.nvim_create_autocmd("FileType", {
    group = vim.api.nvim_create_augroup("fga_treesitter", { clear = true }),
    pattern = "fga",
    callback = function(args)
      pcall(vim.treesitter.start, args.buf, "fga")
    end,
  })

  if vim.fn.executable(cmd[1]) ~= 1 then
    vim.notify(
      ("fga-lsp: %s is not executable; run `make` in %s"):format(cmd[1], vim.fn.fnamemodify(root, ":h")),
      vim.log.levels.WARN
    )

    return
  end

  local config = {
    cmd = cmd,
    filetypes = { "fga" },
    -- fga.mod marks a modular model, and is what bounds name resolution.
    -- Falling back to the repository keeps a single-file model working.
    root_markers = { "fga.mod", ".git" },
    settings = opts.settings or {},
  }

  vim.lsp.config("fga", config)
  vim.lsp.enable("fga")

  -- Store tests and fga.mod keep the yaml filetype, so vim.lsp.enable will
  -- not reach them; they are attached by name instead. vim.lsp.start reuses
  -- an existing client whose config matches, so a model and its tests share
  -- one server and therefore one view of the workspace.
  vim.api.nvim_create_autocmd({ "BufReadPost", "BufNewFile" }, {
    group = vim.api.nvim_create_augroup("fga_lsp_yaml", { clear = true }),
    pattern = { "*.fga.yaml", "*.fga.yml", "fga.mod" },
    callback = function(args)
      if not matches_fga_file(vim.api.nvim_buf_get_name(args.buf)) then
        return
      end

      vim.lsp.start(
        vim.tbl_extend("force", config, {
          name = "fga",
          root_dir = vim.fs.root(args.buf, { "fga.mod", ".git" }) or vim.fn.getcwd(),
        }),
        { bufnr = args.buf }
      )
    end,
  })
end

return M
