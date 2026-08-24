# wtf plan (spike)

Render and validate a profile-driven local environment without changing the
filesystem.

```bash
wtf plan <profile> <instance>
```

> **Spike status:** `wtf plan` is read-only. `wtf up` materializes the
> declared worktrees, env files, port leases, and commands; `wtf down` stops
> recorded processes and releases port leases. Worktrees are intentionally
> retained so uncommitted work is never deleted.

## Setup and run

1. Create a workspace directory and put participating repos or directories in
   it. A service path is relative to that directory (`.` is valid for the
   workspace root) and cannot escape it with `..`.
2. Create `<workspace>/.wtf/workspace.yaml`:

```yaml
# <workspace>/.wtf/workspace.yaml
version: 1

profiles:
  fullstack:
    services: [web, api]

services:
  web:
    dir: web
    env:
      path: .env
      mode: copy
      set:
        PORT: "${port.web_http}"
        NEXT_PUBLIC_API_URL: "http://127.0.0.1:${port.api_http}/"
    ports:
      web_http: {from: 3000, to: 3099}
    up: pnpm exec next dev --port $PORT

  api:
    dir: api
    source: worktree # default; use directory for an existing, shared directory
    env:
      path: app/.env
      mode: copy
      set:
        PORT: "${port.api_http}"
    ports:
      api_http: {from: 8000, to: 8099}
    up: uv run manage.py runserver 0.0.0.0:$PORT
```

3. Run the plan or start the environment from the workspace root **or any
   descendant directory**. WTF searches parent directories for
   `.wtf/workspace.yaml`.

```bash
$ cd <workspace>/api
$ wtf plan fullstack feature/auth
Profile fullstack · instance feature/auth
workspace: <workspace>

web
  dir: <workspace>/web
  source: worktree
  worktree: feature/auth
  port: http (3000-3099)

api
  dir: <workspace>/api
  source: worktree
  worktree: feature/auth
  env: app/.env (copy)
  port: api_http (8000-8099)
  up: uv run python manage.py runserver 0.0.0.0:$PORT
```

## Manifest reference

- `version`: must be `1`.
- `profiles.<name>.services`: ordered list of service names to include.
- `services.<name>.dir`: required path relative to the workspace root; `.`
  selects the root itself.
- `services.<name>.source`: `worktree` (the default) or `directory`. The plan
  labels worktree services with the requested instance; directory services are
  intentionally shared.
- `services.<name>.env`: optional `{path, mode, set}`. `path` is the dotenv
  path relative to the service root; `mode` is `copy` or `symlink`. `set`
  safely upserts values only into copied files and supports `${instance}` and
  `${port.<name>}` interpolation.
- `services.<name>.ports.<name>`: an optional `{from, to}` inclusive range.
  `up` leases a free value from the range for the instance.
- `services.<name>.up`: an opaque application command. WTF does not interpret
  Django, Node, Docker, or any other framework syntax.

Start and stop the instance:

```bash
wtf up fullstack feature/auth
wtf down feature/auth
```

Use `--json` for scripts and agents:

```bash
wtf plan fullstack feature/auth --json
wtf up fullstack feature/auth --json
wtf ps
wtf ps --global
```

Runtime state and command logs are stored under `.wtf/state/`, which is ignored
by Git. `down` preserves created worktrees; use the existing `wtf rm` workflow
when you have finished with a branch.
