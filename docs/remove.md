# Workspace cleanup

Run `wtf remove` or `wtf rm` to open the inline cleanup picker.

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
- `f` toggles force mode, initially OFF. Force ON allows selecting workspaces
  with remaining changes. Turning it OFF clears selections requiring force.
- `a` selects clean workspaces verified in main/master. Press it again to clear those selections.
- Enter opens the review, then Enter confirms removal. Esc returns to the selection; Ctrl+C cancels.

Safety inspection dispatches workspaces in display order, with up to 32 concurrent
inspections and a separate bounded metadata lookup for each eligible workspace.
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
freshly observed as merged into main/master also verifies integration, including
squash merges. A closed PR, matching branch name or stale cached result does not
prove integration. Unknown observations remain warnings. Remote bookmarks reflect
the last fetch; inspection does not fetch or update-stale automatically.

Normal JJ snapshotting captures on-disk edits during inspection. Every deletion
rechecks source changes, workspace identity and resource ownership and invokes
the existing project removal lifecycle. Each result is reported separately if a
multi-selection only partially succeeds.

`wtf remove NAME` keeps direct single-workspace removal. `wtf remove --json`
reports candidates without prompting or removing them. Redirected human output
also reports candidates instead of opening a picker.
