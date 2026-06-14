if vim.b.did_klang_ftplugin then
  return
end
vim.b.did_klang_ftplugin = true

vim.bo.commentstring = "-- %s"
vim.bo.comments = ":--"
vim.bo.expandtab = true
vim.bo.shiftwidth = 4
vim.bo.softtabstop = 4
vim.bo.tabstop = 4
vim.bo.suffixesadd = ".klang"
vim.bo.formatoptions = vim.bo.formatoptions:gsub("t", "")

local snippets = require("klang.snippets")

vim.api.nvim_buf_create_user_command(0, "KlangSnippet", function(opts)
  snippets.insert(opts.args)
end, {
  nargs = 1,
  complete = function()
    return snippets.names()
  end,
  desc = "Insert a Klang snippet",
})

vim.keymap.set("i", "<leader>kf", function()
  snippets.insert("function")
end, { buffer = true, desc = "Insert Klang function snippet" })

vim.keymap.set("i", "<leader>ka", function()
  snippets.insert("alias")
end, { buffer = true, desc = "Insert Klang alias function snippet" })

vim.keymap.set("i", "<leader>kn", function()
  snippets.insert("namespace")
end, { buffer = true, desc = "Insert Klang namespace snippet" })

vim.keymap.set("i", "<leader>km", function()
  snippets.insert("main")
end, { buffer = true, desc = "Insert Klang main snippet" })
