# Jujutsu (jj) support

wtf drives [Jujutsu](https://jj-vcs.github.io/jj/) workspaces with the same commands
it uses for git worktrees. A jj workspace and a git worktree are the same idea — a
sibling checkout of one repo — so `new`, `news`, `sw`, `swg`, `rm`, `rmg`, `clean`,
`port`, and `setup` all work either way, including the automatic env-file linking
and package install.

## How wtf picks a backend

| Repo contains | wtf uses |
|---------------|----------|
| `.git` only | git worktrees |
| `.jj` only | jj workspaces |
| both (colocated) | asks once, then remembers |

Colocation is jj's **default** — `jj git init` creates both `.git` and `.jj`, and
`--no-colocate` is opt-in — so "both present" is the normal shape of a jj repo rather
than a rarity. wtf resolves it in this order:

1. `--vcs git` / `--vcs jj` on the command
2. the `WTF_VCS` environment variable
3. a choice previously saved for this repo
4. an interactive prompt, whose answer is saved
5. no terminal to ask on → inferred from evidence (see below)
6. nothing to infer from → **git**, with a warning on stderr

```
$ wtf new feat/auth
? myrepo is both a git and a jj repo — which should wtf use?
  [1] jj    workspace   (jj manages the working copy here)
  [2] git   worktree
Use which? [1-2] (saved for this repo)
```

The answer is stored per-repo in `~/.wtf/repos.json`, so you are asked once rather
than once per command. Override it any time with `--vcs`, or clear it to be asked
again.

In a non-interactive shell there is nobody to ask, so wtf uses evidence instead of
guessing, in two steps:

1. **Existing checkouts.** A repo holding jj workspaces resolves to jj; one holding
   git worktrees resolves to git. This is backward compatible by construction — a
   repo whose worktrees wtf created with git keeps resolving to git. Evidence for
   both means wtf refuses to guess.
2. **Who owns the working copy.** With no secondary checkouts (a freshly cloned
   repo), git's HEAD still tells you: jj leaves HEAD **detached** whenever it updates
   the working copy, so a detached HEAD means jj is driving. `jj git clone` produces
   this; `git clone` leaves HEAD on a branch.

Without step 2, a fresh `jj git clone` would be handed git worktrees in CI. Only a
colocated repo with no checkouts *and* a git-owned HEAD — the "someone added jj to a
git repo" shape — is genuinely undecidable:

```
$ wtf sw            # colocated, git owns HEAD, no checkouts, piped output
⚠ myrepo is both a git and a jj repo — defaulting to git
  hint: pass --vcs jj or set WTF_VCS=jj
```

For scripting, set it once instead of per-command:

```bash
export WTF_VCS=jj
```

## Workspace names, not branches

jj has no branch-per-checkout: workspaces are **named**, and bookmarks are separate
metadata. wtf keys on the workspace name, which jj guarantees is unique, so it plays
exactly the role a branch name plays under git:

```bash
wtf new feat/auth      # jj workspace add --name feat/auth … -r trunk()
wtf sw auth            # substring match on the workspace name
wtf rm feat/auth       # jj workspace forget + delete the directory
```

**wtf never creates a bookmark.** Bookmarks are a push-time concern and stay yours:

```bash
jj bookmark create feat/auth    # when you want one
jj git push -c @                # or let jj create it at push time
```

A workspace is also findable by any bookmark pointing at its working-copy commit,
so `wtf sw shipit` works once you have made a `shipit` bookmark.

## Listing

Columns are named after what they actually hold, so a jj listing never reads like a
git one:

```
$ wtf sw
WORKSPACE  BOOKMARK  PATH                        CHANGE
default *  —         /code/myrepo                qrqqxqtl
feat/auth  shipit    /code/feat-auth--myrepo     lsuzstxk
```

`CHANGE` is jj's change id, which stays stable as a change is rewritten — more
useful than a commit hash that moves under you. `—` means no bookmark, which is the
normal state.

In a colocated repo, wtf points out checkouts held by the backend you are not
looking at rather than hiding half the repo:

```
$ wtf sw --vcs git
BRANCH      PATH          HEAD
(detached)  /code/myrepo  9955b41
note: 1 jj workspace also exists here — wtf sw --vcs jj
```

`git` reporting `(detached)` for the primary checkout is normal in a colocated repo:
jj owns the working copy and leaves git's HEAD detached.

A `sw` miss under one backend also checks the other:

```
$ wtf sw --vcs git only-in-jj
error: no worktree found matching only-in-jj
note: only-in-jj exists as a jj workspace here — wtf sw --vcs jj only-in-jj
```

## Global commands

`swg`, `rmg`, and `wtf sw -g` span every registered repo, mixing git and jj freely.
Each row carries its own backend, so nothing has to be asked mid-listing:

```
$ wtf swg
▸ myrepo (jj) (/code/myrepo)
  WORKSPACE  BOOKMARK  PATH                     CHANGE
  default *  —         /code/myrepo             qrqqxqtl

  other (git) (/code/other)
  BRANCH   PATH            HEAD
  main *   /code/other     a1b2c3d
```

A colocated repo with no saved preference contributes **both** its git worktrees and
its jj workspaces — they are different directories, and both genuinely exist.
Selecting a row in `rmg` selects its backend too.

## Fetching (`--branch` and `--pr`)

Both work under jj. Fetching goes through the git repo backing the jj store rather
than a plain `git fetch`, because in a `--no-colocate` repo that git repo lives inside
`.jj` where git cannot find it from the working directory:

```bash
wtf new --branch remote-feature    # jj: fetch into the backing repo, then jj git import
wtf new --pr 42
```

`jj git fetch` is deliberately not used: it can only fetch bookmarks, and has no way
to express a refspec like `pull/42/head:pr-42`, which is what forge PR checkout needs.
After the fetch, `jj git import` makes the ref available as a jj revset so the new
workspace can be based on it.

## Base revisions

`--base` takes a jj revset, not just a branch name:

```bash
wtf new feat --base main            # a bookmark
wtf new feat --base 'trunk()'       # a revset
wtf new feat --base qrqqxqtl        # a change id
```

With no `--base`, wtf uses `trunk()` when it resolves to real work, and otherwise
falls back to jj's own default (a sibling of your current working-copy commit). The
fallback matters in a repo with no remote, where `trunk()` resolves to the *root*
commit — basing a workspace there would produce an empty directory.

## Env files and install

This is identical to git, and it is the main reason to use wtf with jj at all: jj
honors `.gitignore`, so `.env` and `node_modules/` are **not** carried into a new
workspace. wtf links and installs them for you:

```
$ wtf new feat/auth
✔ Created workspace at /code/feat-auth--myrepo
  env: .env → symlink
  install: pnpm install
port: PORT=3001
```

`--no-setup`, `--no-env`, `--copy-env`, `--no-install`, and `--no-serve` all behave
as they do under git. Use `--copy-env` for isolated agent workspaces.

## clean

`wtf clean` removes checkouts holding no unlanded work. Since jj has no merged
branch to test, the equivalent is a workspace whose working-copy commit is empty
*and* whose parent is already contained in `trunk()`. Workspaces whose directory was
deleted outside wtf are reported as `prunable`:

```
$ wtf clean --dry-run
~ Would remove feat/auth (prunable)
```

Anything holding real changes is left alone.

## State location

wtf keeps per-repo state (allocated ports, forge cache, watch state) under
`.jj/repo/wtf/` instead of `.git/wtf/`, so every workspace of a repo agrees on one
location — the same guarantee `--git-common-dir` gives under git.

## Requirements

The `jj` binary must be on `PATH`. If a repo needs it and it is missing, wtf says so
rather than guessing. Tested against jj 0.43.

## Known limits

- **PR/forge integration** resolves a workspace to a PR through its bookmarks, so a
  workspace with no bookmark will not match an open PR in `wtf sw --prs`.
- **`wtf clean`** is deliberately conservative under jj: it will not remove a
  workspace whose change is non-empty, even if that change has already landed.
