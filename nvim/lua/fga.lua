-- Neovim wiring for the OpenFGA DSL. See docs/editor-integration.md.
--
-- Build the parser and the server first: `make` at the repository root.

local M = {}

local function plugin_root()
  return vim.fn.fnamemodify(debug.getinfo(1, "S").source:sub(2), ":h:h")
end

-- Store tests and fga.mod stay YAML; they are attached by filename below.
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

-- matches_fga_file reports whether a buffer holds something the server answers for.
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
    root_markers = { "fga.mod", ".git" },
    settings = opts.settings or {},
  }

  vim.lsp.config("fga", config)
  vim.lsp.enable("fga")

  -- vim.lsp.start reuses a matching client, so a model and its tests share one.
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
