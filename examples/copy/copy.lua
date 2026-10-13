local wtf = require("wtf")
local fs = require("wtf.fs")

-- Project policy: change these rules to select your own local files.
local rules = {
  directory = ".wtf/local/copy-example",
  prefixes = { ".env" },
  suffixes = { ".local" },
  files = { ["copy me.md"] = true },
  symlink = { ["preferences.local"] = true },
}

local function selected(name)
  if rules.files[name] then return true end
  for _, prefix in ipairs(rules.prefixes) do
    if name:sub(1, #prefix) == prefix then return true end
  end
  for _, suffix in ipairs(rules.suffixes) do
    if name:sub(-#suffix) == suffix then return true end
  end
  return false
end

return function(event)
  -- Manual invocation in the main checkout must not link a file to itself.
  if event.project.path == event.workspace.path then
    wtf.notify("copy · main workspace; nothing to transfer")
    return
  end
  if not fs.exists(rules.directory, { root = "project" }) then
    wtf.notify("copy · no local source directory; skipped")
    return
  end

  fs.mkdir(rules.directory, { root = "workspace" })
  -- Child copies are ignored too; preserve any existing ignore rules.
  fs.write(".wtf/.gitignore", "*\n", { root = "workspace" })
  local count = 0
  for _, entry in ipairs(fs.list(rules.directory, { root = "project" })) do
    if entry.type == "file" and selected(entry.name) then
      local path = rules.directory .. "/" .. entry.name
      local outcome
      if rules.symlink[entry.name] then
        outcome = fs.symlink(path, path, { from = "project", to = "workspace" })
      else
        outcome = fs.copy(path, path, { from = "project", to = "workspace" })
      end
      count = count + 1
      wtf.notify("copy · " .. entry.name .. " · " .. outcome.status)
    end
  end
  wtf.notify("copy · " .. count .. " local files prepared", "success")
end
