local compose = require("lib.compose")
return function(ctx)
  if compose.active(ctx) then
    compose.test(ctx)
  else
    ctx:stage("test", { "go", "test", "-p", "1", "./internal/core" })
  end
end
