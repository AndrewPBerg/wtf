-- Host-backed module factory. One module instance belongs to one invocation.
return function(wtf, validate)
  local function callback(fn)
    return function(ctx, request, default)
      local event = setmetatable({
        ctx = ctx,
        request = request,
        name = request.name,
        from = request.from,
        path = request.path,
        args = request.args,
        command = request.command,
        surface = request.surface,
        trigger = request.trigger,
        result = request.result,
        error = request.error,
      }, { __index = ctx })

      if default then
        event.default = function(replacement)
          return default(replacement or request)
        end
      end

      return fn(event)
    end
  end

  function wtf.setup(opts)
    if opts == nil then opts = {} end
    validate(opts)
    local registration = { hooks = {}, overrides = {} }
    for name, fn in pairs(opts.hooks or {}) do
      registration.hooks[name] = callback(fn)
    end
    for name, fn in pairs(opts.overrides or {}) do
      registration.overrides[name] = callback(fn)
    end
    return registration
  end

  return wtf
end
