# wtf pr

Checkout a pull request (or merge request) as a new worktree.

## Usage

```bash
wtf pr <number|branch>
```

## How It Works

1. Detects GitHub or GitLab from your `origin` remote URL
2. Resolves the PR by number or branch name (substring match supported)
3. Fetches the PR head ref
4. Creates a new worktree for the PR branch
5. Runs project setup and `on_pr_create` hooks if configured

## Authentication

Reuses existing CLI tokens, no custom credential management:

- **GitHub**: `gh auth token` (install [gh](https://cli.github.com/))
- **GitLab**: `glab auth token` or `GITLAB_TOKEN` env var (install [glab](https://gitlab.com/gitlab-org/cli))

## Examples

```bash
# Checkout PR by number
$ wtf pr 42
✔ Checked out #42 → /code/myrepo--feature-auth

# Checkout PR by branch name
$ wtf pr feature-auth
✔ Checked out #42 → /code/myrepo--feature-auth

# Partial branch match
$ wtf pr auth
✔ Checked out #42 → /code/myrepo--feature-auth
```

## Tab Completion

```bash
# Lists open PRs with number, branch, and author
$ wtf pr <TAB>
#42  feature-auth (alice)
#43  fix-login (bob)
```

Completions are backed by a cached PR list (5-minute TTL) for fast responses.

## PR Hooks

Configure PR-specific hooks in `.wt-forge.toml`:

```toml
[hooks]
on_pr_create = ["echo 'PR worktree ready'"]
on_pr_switch = ["echo 'switched to PR'"]
on_pr_delete = ["echo 'PR worktree removed'"]
```

## See Also

- [ls](ls.md) — List worktrees (with `--prs` for PR status)
- [new](new.md) — Create a worktree for a regular branch
- [.wt-forge.toml](wt-forge-toml.md) — Configuration reference
