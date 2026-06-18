# wtf setup

Run project setup in the current worktree.

## Usage

```bash
wtf setup              # full setup (symlink env files + install)
wtf setup --copy-env   # copy env files instead of symlinking + install
wtf setup --env        # only handle env files
wtf setup --env --copy-env  # only copy env files
wtf setup --install    # only run package install
wtf setup shell        # configure shell integration (one-time)
```

## What It Does

1. **Handles env files** from the main worktree (`.env`, `.env.local`, `.env.development`, `.env.development.local` -- only files that exist in the main worktree), symlinking by default or copying with `--copy-env`
2. **Auto-detects the package manager** from lockfiles and runs install

No config file needed. This is the same setup that runs automatically after `wtf new` and `wtf news`.

## Package Manager Detection

WTF detects the package manager by checking for lockfiles in priority order:

| Lockfile              | Command                           |
|-----------------------|-----------------------------------|
| `pnpm-lock.yaml`      | `pnpm install`                    |
| `bun.lockb`           | `bun install`                     |
| `yarn.lock`           | `yarn install`                    |
| `package-lock.json`   | `npm install`                     |
| `uv.lock`             | `uv sync`                         |
| `poetry.lock`         | `poetry install`                  |
| `requirements.txt`    | `pip install -r requirements.txt` |
| `go.sum`              | `go mod download`                 |
| `Cargo.lock`          | `cargo build`                     |
| `Gemfile.lock`        | `bundle install`                  |
| `composer.lock`       | `composer install`                |
| `pom.xml`             | `mvn install`                     |
| `build.gradle(.kts)`  | `gradle build`                    |
| `packages.lock.json`  | `dotnet restore`                  |
| `mix.lock`            | `mix deps.get`                    |
| `Package.resolved`    | `swift package resolve`           |

First match wins. If no lockfile is found, the install step is skipped.

## Examples

```bash
# Re-run setup in current worktree
$ wtf setup
✔ Setup complete

# Only install packages
$ wtf setup --install
✔ Ran pnpm install

# Only symlink env files
$ wtf setup --env
✔ .env symlinked

# Only copy env files for an isolated agent worktree
$ wtf setup --env --copy-env
✔ .env copied
```

## Shell Integration

Shell integration is configured via `wtf setup shell`:

```bash
$ wtf setup shell
Detected shell: zsh
RC file: /home/user/.zshrc
Will add: eval "$(wtf init zsh)"
Proceed? [y/N] y
✔ Added to /home/user/.zshrc. Restart your shell or run: source /home/user/.zshrc
✔ Tab completions included automatically via wtf init
```

This single line handles both the shell wrapper (for `wtf sw` / `wtf news` to `cd` automatically) and tab completions for all commands.

If already configured:

```bash
$ wtf setup shell
✔ Shell integration already configured in /home/user/.zshrc
✔ Tab completions included automatically via wtf init
```
