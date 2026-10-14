local wtf = require("wtf")
local fs = require("wtf.fs")
local exec = require("wtf.exec")
local env = require("wtf.env")

-- This sandbox is local and ignored. Fixed filenames make repeated runs safe.
return function(event, surface)
  local directory = ".wtf/local/hello-world/" .. surface
  fs.mkdir(directory)
  fs.write(".wtf/.gitignore", "*\n") -- preserve any existing ignore rules

  local settings = env.merge(env.select({ "WTF_HELLO_LABEL" }), {
    WTF_HELLO_LABEL = env.get("WTF_HELLO_LABEL", "Lua sandbox"),
    WTF_HELLO_SURFACE = surface,
    WTF_HELLO_UNUSED = false, -- unset in the child, without changing your shell
  })

  local command = exec.run({ "sh", "-c", [[
    printf 'hello world from %s: %s\n' "$WTF_HELLO_SURFACE" "$WTF_HELLO_LABEL"
    printf '%s' "${WTF_HELLO_UNUSED-unset}" >&2
  ]] }, {
    cwd = directory,
    env = settings,
    inherit_env = false,
    timeout_seconds = 5,
  })
  assert(command.ok and command.stderr == "unset")

  local greeting = directory .. "/greeting.txt"
  fs.write(greeting, command.stdout, { replace = true })
  -- This independent copy preserves your edits on subsequent invocations.
  local copy = fs.copy(greeting, directory .. "/kept-copy.txt")
  local link = fs.symlink(greeting, directory .. "/latest", { replace = true })
  fs.copy(directory .. "/latest", directory .. "/latest-copy", {
    symlinks = "preserve",
    replace = true,
  })
  assert(fs.read(directory .. "/latest") == fs.read(greeting))
  assert(fs.stat(directory .. "/latest").type == "symlink")

  local probe = directory .. "/temporary-probe.txt"
  fs.write(probe, "safe to remove", { replace = true })
  assert(fs.read(probe) == "safe to remove")
  fs.remove(probe)
  assert(fs.remove(probe).status == "missing")

  local rows = event.result or {}
  local count = rows.path and 1 or #rows
  local report = command.stdout
    .. "surface=" .. surface .. "\n"
    .. "listed_workspaces=" .. count .. "\n"
    .. "independent_copy=" .. copy.status .. "\n"
    .. "symlink=" .. link.status .. "\n"
  fs.write(directory .. "/report.txt", report, { replace = true })

  wtf.notify("hello world · " .. surface .. " · " .. #fs.list(directory)
    .. " local artifacts · copy " .. copy.status .. " · link " .. link.status, "success")
end
