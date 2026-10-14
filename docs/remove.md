# Workspace cleanup

Run `wtf remove` or `wtf rm` to open the inline cleanup picker.

Inside a repository, removal uses that repository. From a parent directory such
as `Work`, it includes registered repositories beneath that directory, just like
`wtf sw`. No `--global` flag is needed. An explicit name must be unique within
that scope; use the workspace ID or run inside the intended repository when names
are repeated. Each selected repository's removal hooks and safety checks still run.

Listing reconciles confirmed missing, idle workspace registrations. It forgets
their JJ registration, removes only their stale Git administrative entry, and
drops them from WTF's registry, allowing their names and paths to be reused.
This cleanup does not remove source directories. In-flight operations, resource
claims, Git worktree locks, broken symlinks, and access errors prevent pruning.
Other unavailable paths are hidden from normal `ls` and removal candidates;
`wtf ls --all --json` can still inspect retained records with `exists: false`.

The `AGE` column shows time since first WTF registration in whole seconds (`S`),
minutes (`M`), hours (`H`), days (`D`), or weeks (`W`). Legacy records without a
timestamp show `?`. Age is informational and does not affect removal eligibility.
JSON candidates include `created_at` when known.

Workspace names link to their exact Zed folder when terminal links are enabled;
the full path remains in JSON. If the highlighted workspace has a description
set with `wtf new PATH -m TEXT`, it appears below the rows without replacing the
cleanup warning. Descriptions are also visible during the removal review.

- Arrow keys move through workspaces, with pagination when needed.
- `j`/`k` move down/up and accept counts such as `4j`/`4k`, wrapping through the list.
- `gg` goes to the top of the current page; `G` goes to its bottom. Left/Right, `h`/`l` (also `H`/`L`), and PageUp/PageDown move between pages. These motions also work in the review.
- Space selects or clears a workspace.
- `/` fuzzy-searches names and messages, ignoring case, with matching characters
  highlighted in the active theme's accent color and underlined. Enter applies
  the filter and returns to navigation; Esc clears it. Backspace edits the query
  and Ctrl+U clears the input. Typing in search never triggers picker actions.
  Selections survive filtering; `a` acts on matching rows only. Review always
  shows every selected workspace, including those hidden by the filter.
- `f` toggles force mode, initially OFF. Force ON allows selecting workspaces
  with remaining changes. Turning it OFF clears selections requiring force.
- `a` selects clean workspaces verified in main/master. Press it again to clear those selections.
- Enter opens the review, then Enter confirms removal. Esc returns to the selection; Ctrl+C cancels.

Safety inspection dispatches workspaces in display order, with up to 32 concurrent
local inspections. Workspaces share the existing successful GitHub observation
while it is within the two-minute metadata lifetime. Only missing or expired
observations require targeted GraphQL discovery, without CI hydration.
Results appear as they finish, without waiting for earlier rows. Completed rows
can be selected and reviewed while other rows are checking; checking rows cannot
be selected.

The picker distinguishes work included in main/master from revisions outside it.
It also warns about changed files. Current and adopted workspaces are protected,
and remaining changes outside main/master block removal.

Force mode discards remaining source changes when removal is confirmed. The
review shows Force ON and how many selected workspaces have remaining changes.
Change the mode on the selection screen; it cannot change during review. Force
does not bypass protected/current workspace, identity, inspection failure,
resource ownership, or project lifecycle guards. `a` still selects only clean,
verified workspaces. Direct named removal keeps its normal safety checks.

Integration is verified through JJ ancestry against the available main/master
bookmark, preferring origin's last-fetched bookmark. An exact GitHub PR head
observed as merged into main/master within the metadata lifetime also verifies
integration, including squash merges. A closed PR, matching branch name or stale cached result does not
prove integration. Unknown observations remain warnings. Remote bookmarks reflect
the last fetch; inspection does not fetch or update-stale automatically.

Normal JJ snapshotting captures on-disk edits during inspection. Every deletion
rechecks source changes, workspace identity and resource ownership and invokes
the existing project removal lifecycle. The picker carries its exact-revision
assessment into deletion; confirmation makes no GitHub requests. Source changes
made after inspection or by removal hooks cannot use the earlier integration
proof. Force removal also uses local checks without provider requests.
Each result is reported separately if a
multi-selection only partially succeeds.

`wtf remove NAME` keeps direct single-workspace removal. `wtf remove --json`
reports candidates without prompting or removing them. Redirected human output
also reports candidates instead of opening a picker.
