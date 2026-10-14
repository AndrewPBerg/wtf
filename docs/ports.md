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
