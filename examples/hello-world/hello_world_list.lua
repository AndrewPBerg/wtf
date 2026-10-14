local hello_world = require("lib.hello_world")

return function(event)
  hello_world(event, "list")
end
