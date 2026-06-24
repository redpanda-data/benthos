local M = {}

local function plugin_dir()
  local source = debug.getinfo(1, "S").source:sub(2)
  return vim.fn.fnamemodify(source, ":h:h:h")
end

M.check = function()
  vim.health.start("bloblang2")

  -- Check Neovim version.
  if vim.fn.has("nvim-0.10") == 1 then
    vim.health.ok("Neovim >= 0.10")
  else
    vim.health.error("Neovim >= 0.10 required for vim.treesitter.language.add()")
  end

  local dir = plugin_dir()

  -- Check tree-sitter parser.
  local so_path = dir .. "/parser/bloblang2.so"
  local dylib_path = dir .. "/parser/bloblang2.dylib"
  if vim.fn.filereadable(so_path) == 1 then
    vim.health.ok("Tree-sitter parser found: " .. so_path)
  elseif vim.fn.filereadable(dylib_path) == 1 then
    vim.health.ok("Tree-sitter parser found: " .. dylib_path)
  else
    vim.health.warn("Tree-sitter parser not found. Run 'task parser' in " .. dir)
  end

  -- Check LSP binary.
  local lsp_path = dir .. "/bin/bloblang2-lsp"
  if vim.fn.executable(lsp_path) == 1 then
    vim.health.ok("LSP binary found: " .. lsp_path)
  else
    vim.health.warn("LSP binary not found. Run 'task lsp' in " .. dir)
  end
end

return M
