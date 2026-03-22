# .wt-forge.toml

Project-level configuration file for WorkTreeForge. Place it in your repository root.

## Full Example

```toml
[worktree]
root_path = "/code/trees"
default_base = "develop"

[env]
strategy = "symlink"    # "symlink" | "copy" | "none"
files = [".env", ".env.local", ".env.development"]

[[setup]]
name = "install"
run = "pnpm install"

[[setup]]
name = "migrate"
run = "pnpm db:migrate"
if = "file exists 'prisma/schema.prisma'"

[[setup]]
name = "seed"
run = "pnpm db:seed"
if = "branch contains 'feature'"

[hooks]
on_create = ["echo 'worktree created'"]
on_switch = ["echo 'switched worktree'"]
on_remove = ["echo 'worktree removed'"]
```

## Sections

### `[worktree]`

| Field | Type | Description |
|---|---|---|
| `root_path` | string | Custom root directory for worktrees |
| `default_base` | string | Default base branch (overrides `--base` default) |

### `[env]`

| Field | Type | Default | Description |
|---|---|---|---|
| `strategy` | string | `""` (no-op) | How to handle env files: `symlink`, `copy`, or `none` |
| `files` | string[] | `.env`, `.env.local`, `.env.development`, `.env.development.local` | Files to handle |

**Strategies:**
- `symlink` — Creates relative symlinks from the new worktree to the main worktree. Only existing files are linked.
- `copy` — Copies files from the main worktree. Only existing files are copied.
- `none` — Does nothing.

### `[[setup]]`

Ordered list of setup steps. Each runs in the worktree directory.

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | no | Human-readable step name |
| `run` | string | yes | Shell command to execute |
| `if` | string | no | Condition — step runs only if true |

**Conditions (if-DSL):**

| Syntax | Description |
|---|---|
| `branch contains '<value>'` | True if the branch name contains the value |
| `file exists '<path>'` | True if the file exists (relative to worktree) |
| `env VAR is set` | True if the environment variable is non-empty |
| _(empty)_ | Always true |

### `[hooks]`

Lifecycle hooks — shell commands run at specific events.

| Field | Type | Description |
|---|---|---|
| `on_create` | string[] | Run after `wtf new` creates a worktree |
| `on_switch` | string[] | Run after `wtf sw` switches to a worktree |
| `on_remove` | string[] | Run before `wtf rm` removes a worktree |

## Behavior Without Config

When no `.wt-forge.toml` exists, `wtf new` still auto-detects the package manager from lockfiles and runs install.

## Package Manager Detection

Lockfiles are checked in priority order:

1. `pnpm-lock.yaml` → `pnpm install`
2. `bun.lockb` → `bun install`
3. `yarn.lock` → `yarn install`
4. `package-lock.json` → `npm install`
5. `uv.lock` → `uv sync`
6. `pyproject.toml` → `uv sync`
