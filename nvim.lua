-- vim.lsp.start({
-- 	name = "brainrot-lsp",
-- 	cmd = { "./bin/emoji-lsp" },
-- 	root_dir = vim.fn.getcwd(),
-- })
vim.lsp.start({
    name = "redpanda-connect-lsp",
    cmd = vim.lsp.rpc.connect("127.0.0.1", 8085),
    root_dir = vim.fn.getcwd(),
})
