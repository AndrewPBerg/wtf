return function(ctx, request)
  print("WTF workspace: " .. ctx.workspace.name .. " (" .. ctx.workspace.path .. ")")
  ctx:stage("workspace-check", { "go", "version" }, {
    inputs = { "go.mod" },
    reuse = true,
  })
end
