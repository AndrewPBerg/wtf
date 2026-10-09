local M = {}
local manifest = "examples/compose/compose.yaml"
local ownership_check = "examples/compose/check-owner.sh"

local function claim(ctx, kind, name)
  for _, item in ipairs(ctx:claims()) do
    if item.kind == kind and item.name == name then return item.value end
  end
end

local function settings(ctx)
  local project = claim(ctx, "compose", "sandbox")
  local endpoint = claim(ctx, "port", "cache")
  ctx:require(project == "wtf-" .. ctx.workspace.id, "Compose claim does not match this workspace")
  ctx:require(endpoint ~= nil, "Compose port claim is missing; inspect the workspace before cleanup")
  local port = endpoint:match(":(%d+)$")
  ctx:require(port ~= nil, "Compose port claim is invalid")
  return project, { WTF_DEMO_PORT = port, WTF_DEMO_ID = ctx.workspace.id }
end

local function command(project, ...)
  local argv = { "docker", "compose", "-f", manifest, "--project-name", project }
  for _, arg in ipairs({ ... }) do table.insert(argv, arg) end
  return argv
end

function M.active(ctx) return claim(ctx, "compose", "sandbox") ~= nil end

function M.start(ctx)
  ctx:require(ctx:exists(manifest), "Create from a revision containing the Compose recipe; use -f @")
  if not claim(ctx, "port", "cache") then
    local selected = nil
    -- This allocation policy is project code, not part of the Go substrate.
    for port = 16379, 16442 do
      if not ctx:listening(port) then
        local value, err = ctx:claim_port("cache", port)
        if value then selected = value; break end
        ctx:require(err == "claim_conflict", "Port claim failed: " .. tostring(err))
      end
    end
    ctx:require(selected ~= nil, "No demo port available in the configured range")
  end
  local project = "wtf-" .. ctx.workspace.id
  ctx:claim("compose", "sandbox", project)
  local _, env = settings(ctx)
  ctx:stage("compose-up", command(project, "up", "-d", "--wait", "--pull", "never"), {
    env = env,
    inputs = { manifest, ownership_check },
    check = command(project, "exec", "-T", "cache", "redis-cli", "ping"),
    reuse = true,
  })
end

function M.test(ctx)
  local project, env = settings(ctx)
  ctx:stage("cache-test", command(project, "exec", "-T", "cache", "redis-cli", "ping"), { env = env })
end

function M.stop(ctx)
  if not M.active(ctx) then
    ctx:release("port", "cache")
    return
  end
  local project, env = settings(ctx)
  ctx:require(ctx:check({ "sh", ownership_check, project, ctx.workspace.id }),
    "Docker is unavailable or a container's ownership does not match; cleanup is blocked")
  ctx:stage("compose-down", command(project, "down", "--volumes", "--remove-orphans"), { env = env })
  ctx:release("compose", "sandbox")
  ctx:release("port", "cache")
end

return M
