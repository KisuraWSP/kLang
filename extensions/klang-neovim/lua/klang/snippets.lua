local M = {}

local snippet_lines = {
  ["function"] = {
    "function Name(value : Int) : Int {",
    "    return value;",
    "}",
  },
  alias = {
    "alias function ArrayList[T: Any](data: T, length: Int, capacity: Int, allocator = .DEFAULT) : type {",
    "    [new] {",
    "        allocator.region = get_default_procces_allocator(#region(100, T), #sizeof(capacity));",
    "    }",
    "",
    "    [delete] {",
    "        allocator.free = free_all_allocator(.{});",
    "    }",
    "",
    "    #extend {",
    "        function get_length() : Int {",
    "            return this.length;",
    "        }",
    "    }",
    "}",
  },
  namespace = {
    "namespace App {",
    "    function Process() : Int {",
    "        return 0;",
    "    }",
    "}",
  },
  main = {
    "#set_entry_point_to_here",
    "function Main() : Int {",
    "    print(\"hello from Klang\");",
    "    return 0;",
    "}",
  },
  match = {
    "if value == {",
    "    case 0:",
    "        print(\"zero\");",
    "    case:",
    "        print(\"default\");",
    "}",
  },
}

function M.names()
  local names = {}
  for name, _ in pairs(snippet_lines) do
    table.insert(names, name)
  end
  table.sort(names)
  return names
end

function M.insert(name)
  local lines = snippet_lines[name]
  if not lines then
    vim.notify("Unknown Klang snippet: " .. tostring(name), vim.log.levels.ERROR)
    return
  end

  local row = vim.api.nvim_win_get_cursor(0)[1]
  vim.api.nvim_buf_set_lines(0, row - 1, row - 1, false, lines)
  vim.api.nvim_win_set_cursor(0, { row, 0 })
  vim.cmd("startinsert")
end

return M
