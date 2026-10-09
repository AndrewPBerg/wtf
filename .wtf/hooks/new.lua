return function(ctx, request, default)
  ctx:hook("guard", request)              -- may block; no workspace exists yet
  return default(request)                -- native creation + shared init lifecycle
end
