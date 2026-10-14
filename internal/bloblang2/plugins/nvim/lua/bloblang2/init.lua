local M = {}

--- Resolve the root directory of this plugin.
local function plugin_dir()
  local source = debug.getinfo(1, "S").source:sub(2) -- strip leading @
  -- .../plugins/nvim/lua/bloblang2/init.lua -> .../plugins/nvim
  return vim.fn.fnamemodify(source, ":h:h:h")
end

local _dir = plugin_dir()

--- Default options.
local defaults = {
  lsp = {
    cmd = nil, -- defaults to <plugin>/bin/bloblang2-lsp
    enabled = true,
  },
}

--- Merged user options.
local opts = {}

--- Register the tree-sitter parser and enable highlighting.
local function setup_treesitter()
  local parser_path = _dir .. "/parser/bloblang2.so"
  if vim.fn.filereadable(parser_path) == 0 then
    -- Try .dylib for macOS builds.
    parser_path = _dir .. "/parser/bloblang2.dylib"
  end
  if vim.fn.filereadable(parser_path) == 0 then
    vim.notify("[bloblang2] tree-sitter parser not found. Run 'task parser' in the plugin directory.", vim.log.levels.WARN)
    return
  end

  vim.treesitter.language.add("bloblang2", {
    path = parser_path,
    filetype = "blobl2",
  })
end

--- Start the LSP client for a buffer.
local function start_lsp(buf)
  if not opts.lsp.enabled then
    return
  end

  local cmd = opts.lsp.cmd
  if not cmd then
    local bin = _dir .. "/bin/bloblang2-lsp"
    if vim.fn.executable(bin) == 0 then
      vim.notify("[bloblang2] LSP binary not found. Run 'task lsp' in the plugin directory.", vim.log.levels.WARN)
      return
    end
    cmd = { bin }
  end

  vim.lsp.start({
    name = "bloblang2-lsp",
    cmd = cmd,
    root_dir = vim.fs.root(buf, { ".git" }) or vim.fn.getcwd(),
  })
end

--- Setup the bloblang2 plugin.
---@param user_opts? table
function M.setup(user_opts)
  opts = vim.tbl_deep_extend("force", defaults, user_opts or {})

  setup_treesitter()

  vim.api.nvim_create_autocmd("FileType", {
    pattern = "blobl2",
    callback = function(args)
      -- Enable tree-sitter highlighting.
      vim.treesitter.start(args.buf, "bloblang2")

      -- Start the LSP client.
      start_lsp(args.buf)
    end,
  })
end

return M
