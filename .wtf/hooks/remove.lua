return function(ctx, request, default)
  ctx:require(ctx:check({ "sh", "-ec", 'changes=$(jj diff --summary); test -z "$changes"' }),
    "JJ could not verify a clean workspace; inspect stale state or source changes before teardown")
  ctx:hook("stop", request)
  return default(request)
end
