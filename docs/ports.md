# Port claims

Port claims coordinate named values among workspaces in one JJ repository:

```sh
wtf ports claim web --port 3000
wtf ports list -j
wtf ports release web
```

Ports must be 1–65535. A claim records ownership of `127.0.0.1:PORT`; it does not
bind a socket, choose a free port, or launch a service. Cross-repository leases
are not implemented. Project Lua owns availability/retry policy and can use
`wtf.resources.listening(port)`, `claim_port(name, port)`, and `release(kind,name)`.

Releasing a claim does not stop a process. Stop project services explicitly in
removal callbacks, then release their claims. Ownership and cleanup state are
checked during workspace removal. Read [hooks](hooks.md) and [state](state.md).

## Registry scope and ownership

All workspaces sharing the same JJ repository share its
`.jj/repo/wtf-v1-pilot/state.json`. Claims are keyed by workspace ID, kind, and
name. Registry updates take a file lock and publish state atomically, including
when separate WTF processes compete. The global repository index is not a global
port registry: two independent repositories can claim the same port.

Another workspace cannot claim the same port value, even under a different name.
Repeating the same named claim with the same value succeeds. Changing its value
requires releasing it first. One workspace can hold the same port under multiple
names; releasing one name leaves the other claim intact. `resources.list()` lists
only the current workspace's claims. Release affects only its own named claim
and succeeds when that claim is already absent.

Claims persist after a hook or service exits. They have no TTL or process-health
tracking. Successful workspace removal clears its port claims, but does not stop
services; project teardown must do that before removal.

## Dynamic selection in Lua

Register this callback in `.wtf/config.toml`:

```toml
version = 1

[new]
after = ["allocate_web_port"]
```

Place it in `.wtf/hooks/allocate_web_port.lua`. A new workspace exists by the
`after` phase, so the callback can claim resources for its identity.

```lua
local wtf = require("wtf")
local resources = wtf.resources

local function choose_port()
  -- Preserve an existing assignment when preparation is retried.
  for _, claim in ipairs(resources.list()) do
    if claim.kind == "port" and claim.name == "web" then
      local port = tonumber(claim.value:match("^127%.0%.0%.1:(%d+)$"))
      wtf.policy.require(port ~= nil, "Invalid existing web port claim")
      return port
    end
  end

  for candidate = 3000, 3099 do
    if not resources.listening(candidate) then
      local port, code = resources.claim_port("web", candidate)
      if port then
        return port
      end
      wtf.policy.require(code == "claim_conflict",
        "Cannot claim web port: " .. tostring(code))
    end
  end
  wtf.policy.block("No available web port in 3000-3099")
end

return function(event)
  local port = choose_port()
  wtf.fs.write(".wtf/web.env", "PORT=" .. tostring(port) .. "\n", {
    replace = true,
  })
  wtf.notify("Web port: " .. tostring(port))
end
```

Your launch script must read `.wtf/web.env` and pass `PORT` to the actual service.
Writing that file neither changes the parent shell environment nor starts a
service. The filesystem path is relative to the active workspace. The same
callback can be invoked from a registered preparation hook for existing workspaces.

`listening()` attempts a TCP connection to IPv4 loopback with a 200 ms timeout.
False means the connection failed; it is not a guarantee that binding will work.
`claim_port()` returns the port on success or `nil, code` on a registry error;
invalid port arguments raise a typed Lua error. Retry only `claim_conflict`,
and propagate other failures.

There is a race between checking availability and the service binding its socket.
Claims coordinate cooperating workspaces in this repository, while the operating
system decides whether a service can bind. If startup reports an address conflict,
project code must release its assignment, select another candidate, update the
configuration, and retry with a bound. A listening socket alone does not prove
that it belongs to your service. Check startup success and service identity through
your project supervisor. WTF does not supervise background services; Lua command
helpers clean up their process groups on Linux.

## Focused stress coverage

`internal/core/port_claim_test.go` exercises 32 concurrent claimants across eight
claim/release rounds, concurrent distinct writes, eight separate processes
competing for eight ports, and the Lua availability/conflict/retry path. It also
checks that claiming does not bind and releasing does not stop a live listener.
These are local registry and Lua tests, not a soak test of a project's real service
supervisor or a machine-global allocation guarantee.
