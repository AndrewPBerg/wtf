local args = require("lib.args")

return function(ctx, request)
  args.allow(ctx, request, { "--compose", "--test" })
  ctx:require(request.name ~= "default" and request.name ~= "main",
    "This project reserves default/main; choose a feature workspace name")
  ctx:require(ctx:exists("go.mod"), "Run this project's new hook from the WTF Go repository")
  ctx:require(ctx:check({ "go", "version" }), "Go must be available before creating a workspace")
  if args.has(request, "--compose") then
    ctx:require(ctx:exists("examples/compose/compose.yaml"), "The Compose recipe is missing")
    ctx:require(ctx:check({ "docker", "compose", "version" }), "Docker Compose is required for --compose")
    ctx:require(ctx:check({ "docker", "info", "--format", "{{.ServerVersion}}" }),
      "Docker must be running before creating a Compose workspace")
  end
end
