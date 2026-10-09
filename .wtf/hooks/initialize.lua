local args = require("lib.args")

return function(ctx, request)
  args.allow(ctx, request, { "--compose", "--test" })
  ctx:hook("prepare", request)
  if args.has(request, "--compose") then ctx:hook("start", request) end
  if args.has(request, "--test") then ctx:hook("test", request) end
end
