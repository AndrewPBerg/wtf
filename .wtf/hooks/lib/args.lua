local M = {}

function M.has(request, flag)
  for _, arg in ipairs(request.args or {}) do
    if arg:lower() == flag:lower() then return true end
  end
  return false
end

function M.allow(ctx, request, flags)
  local known = {}
  for _, flag in ipairs(flags) do known[flag:lower()] = true end
  for _, arg in ipairs(request.args or {}) do
    ctx:require(known[arg:lower()] == true, "Unknown project hook argument: " .. arg)
  end
end

return M
