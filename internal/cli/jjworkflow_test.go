package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/jj"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/AndrewPBerg/wtf/internal/ui"
	"github.com/AndrewPBerg/wtf/internal/vcs"
)

// jjTestManager returns a jj manager plus a repo with the jj backend pinned, so
// dispatch is settled and the tests exercise the workflow rather than the prompt.
func jjTestManager(t *testing.T) (vcs.Manager, string) {
	t.Helper()
	resetVCSState(t)
	root := initCLITestJJRepo(t)
	require.NoError(t, config.SetVCSPref(root, vcs.KindJJ))
	t.Chdir(root)
	return newManager(vcs.KindJJ), root
}

func TestJJ_NewCreatesWorkspaceAndSymlinksEnv(t *testing.T) {
	mgr, root := jjTestManager(t)

	cmd, stdout, stderr := newTestCmd("")
	runner := setup.NewRunner()
	// Installing is out of scope here; env handling is the part under test.
	newNoInstall = true
	newNoServe = true
	t.Cleanup(func() { newNoInstall = false; newNoServe = false })

	require.NoError(t, runNew(cmd, "feat/auth", "main", mgr, runner, false))

	wsPath := vcs.WorktreePath(root, filepath.Base(root)+"/feat/auth")
	assert.DirExists(t, wsPath)

	// The output names a workspace, not a worktree, so the backend is obvious.
	assert.Contains(t, stdout.String()+stderr.String(), "Created workspace at")
	assert.FileExists(t, filepath.Join(wsPath, ".git", vcs.JJGitDiffMarker),
		"jj workspaces should expose editor Git diffs by default")

	canonical, err := canonicalWorkspaceName(filepath.Base(root), "feat/auth")
	require.NoError(t, err)
	// Assert both the real jj registration and wtf's manager view: the native
	// workspace name is the canonical scoped identity, not the request alone.
	realList, err := (&jj.RealExecutor{}).Run(root, "workspace", "list", "--ignore-working-copy")
	require.NoError(t, err)
	assert.Contains(t, realList, canonical)
	listed, err := mgr.List(root)
	require.NoError(t, err)
	var got vcs.Worktree
	for _, item := range listed {
		if item.Path == wsPath {
			got = item
		}
	}
	assert.Equal(t, canonical, got.Name)
	assert.Equal(t, canonical, got.NativeName)
	assert.Equal(t, canonical, got.Branch)

	stateDir, err := mgr.StateDir(root)
	require.NoError(t, err)
	marker, err := identity.ReadRepositoryID(stateDir)
	require.NoError(t, err)
	store, err := identity.DefaultStore()
	require.NoError(t, err)
	state, err := store.Load()
	require.NoError(t, err)
	require.Len(t, state.Repositories, 1)
	require.Len(t, state.Workspaces, 1)
	assert.Equal(t, state.Repositories[0].ID, marker)
	assert.Equal(t, marker, state.Workspaces[0].RepositoryID)
	assert.Equal(t, canonical, state.Workspaces[0].Name)
	assert.Equal(t, canonical, state.Workspaces[0].NativeName)

	// The project files came across...
	assert.FileExists(t, filepath.Join(wsPath, "a.txt"))
	// ...and .env did not, because jj honors .gitignore — which is precisely why
	// wtf has to link it in.
	linked := filepath.Join(wsPath, ".env")
	assert.FileExists(t, linked)
	data, err := os.ReadFile(linked)
	require.NoError(t, err)
	assert.Equal(t, "SECRET=1\n", string(data))

	info, err := os.Lstat(linked)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, ".env should be symlinked by default")
}

func TestJJ_NewsCreatesCanonicalNativeWorkspace(t *testing.T) {
	mgr, root := jjTestManager(t)
	newNoInstall = true
	newNoServe = true
	t.Cleanup(func() { newNoInstall = false; newNoServe = false })

	cmd, stdout, _ := newTestCmd("")
	t.Chdir(root)
	require.NoError(t, dispatchNew(cmd, []string{"feat/news"}, "", "", "", true))
	wsPath := strings.TrimSpace(stdout.String())
	canonical, err := canonicalWorkspaceName(filepath.Base(root), "feat/news")
	require.NoError(t, err)
	realList, err := (&jj.RealExecutor{}).Run(root, "workspace", "list", "--ignore-working-copy")
	require.NoError(t, err)
	assert.Contains(t, realList, canonical)
	listed, err := mgr.List(root)
	require.NoError(t, err)
	var found bool
	for _, item := range listed {
		if item.Path == wsPath {
			found = true
			assert.Equal(t, canonical, item.Name)
			assert.Equal(t, canonical, item.NativeName)
		}
	}
	assert.True(t, found, "news path should be a real listed jj workspace")
}

func TestJJ_NewCanOptOutOfGitDiffMetadata(t *testing.T) {
	mgr, root := jjTestManager(t)
	newNoGitDiff = true
	t.Cleanup(func() { newNoGitDiff = false })

	cmd, _, _ := newTestCmd("")
	require.NoError(t, runNew(cmd, "plain", "main", mgr, nil, false))

	wsPath := vcs.WorktreePath(root, filepath.Base(root)+"/plain")
	assert.NoDirExists(t, filepath.Join(wsPath, ".git"))
}

func TestJJ_NewHonorsGitDiffEnvironmentOptOut(t *testing.T) {
	mgr, root := jjTestManager(t)
	t.Setenv("WTF_JJ_GIT_DIFF", "0")

	cmd, _, _ := newTestCmd("")
	require.NoError(t, runNew(cmd, "env-plain", "main", mgr, nil, false))

	wsPath := vcs.WorktreePath(root, filepath.Base(root)+"/env-plain")
	assert.NoDirExists(t, filepath.Join(wsPath, ".git"))
}

func TestJJ_NewCopyEnvWritesRealFile(t *testing.T) {
	mgr, root := jjTestManager(t)

	newCopyEnv = true
	newNoInstall = true
	newNoServe = true
	t.Cleanup(func() { newCopyEnv = false; newNoInstall = false; newNoServe = false })

	cmd, _, _ := newTestCmd("")
	require.NoError(t, runNew(cmd, "agent-1", "main", mgr, setup.NewRunner(), false))

	linked := filepath.Join(vcs.WorktreePath(root, filepath.Base(root)+"/agent-1"), ".env")
	info, err := os.Lstat(linked)
	require.NoError(t, err)
	assert.Zero(t, info.Mode()&os.ModeSymlink, "--copy-env must write a real file")
}

func TestJJ_SwFindsWorkspaceByName(t *testing.T) {
	mgr, root := jjTestManager(t)

	_, err := mgr.Add(root, "feat/auth", "main")
	require.NoError(t, err)

	cmd, stdout, _ := newTestCmd("")
	require.NoError(t, runSw(cmd, "auth", mgr))

	// sw prints the path on stdout for the shell wrapper to cd into.
	assert.Contains(t, stdout.String(), "feat-auth--"+filepath.Base(root))
}

func TestJJ_RmRemovesWorkspaceDirectory(t *testing.T) {
	mgr, root := jjTestManager(t)

	wsPath, err := mgr.Add(root, "feat", "main")
	require.NoError(t, err)
	require.DirExists(t, wsPath)

	cmd, stdout, _ := newTestCmd("")
	require.NoError(t, runRm(cmd, "feat", mgr))

	assert.Contains(t, stdout.String(), "Removed workspace for")
	// jj workspace forget leaves the directory behind; wtf must delete it.
	assert.NoDirExists(t, wsPath)
}

func TestJJ_RmRefusesMainWorkspace(t *testing.T) {
	mgr, _ := jjTestManager(t)

	cmd, _, _ := newTestCmd("")
	err := runRm(cmd, "default", mgr)
	assert.ErrorIs(t, err, vcs.ErrMainWorktree)
}

func TestJJ_LsShowsWorkspaceColumns(t *testing.T) {
	mgr, root := jjTestManager(t)

	_, err := mgr.Add(root, "feat", "main")
	require.NoError(t, err)

	// Force the non-interactive table path.
	prev := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	t.Cleanup(func() { stdinIsTTY = prev })

	cmd, stdout, _ := newTestCmd("")
	require.NoError(t, runLs(cmd, mgr))

	out := stdout.String()
	// jj has no branch per checkout, so the columns say what they actually hold.
	assert.Contains(t, out, "WORKSPACE")
	assert.Contains(t, out, "BOOKMARK")
	assert.Contains(t, out, "CHANGE")
	assert.NotContains(t, out, "BRANCH")
	assert.Contains(t, out, "feat")
}

func TestJJ_LsShowsBookmarkWhenPresent(t *testing.T) {
	mgr, root := jjTestManager(t)

	_, err := mgr.Add(root, "feat", "main")
	require.NoError(t, err)

	prev := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	t.Cleanup(func() { stdinIsTTY = prev })

	cmd, stdout, _ := newTestCmd("")
	require.NoError(t, runLs(cmd, mgr))
	// No bookmark exists yet, and wtf never creates one.
	assert.Contains(t, stdout.String(), "—")

	// Once the user makes one themselves, it shows up.
	wts, err := mgr.List(root)
	require.NoError(t, err)
	var target string
	for _, wt := range wts {
		if wt.Branch == "feat" {
			target = wt.Path
		}
	}
	require.NotEmpty(t, target)
	_, err = (&jj.RealExecutor{}).Run(target, "bookmark", "create", "shipit", "-r", "@")
	require.NoError(t, err)

	cmd2, stdout2, _ := newTestCmd("")
	require.NoError(t, runLs(cmd2, mgr))
	assert.Contains(t, stdout2.String(), "shipit")
}

func TestJJ_CleanRemovesPrunableWorkspace(t *testing.T) {
	mgr, root := jjTestManager(t)

	wsPath, err := mgr.Add(root, "feat", "main")
	require.NoError(t, err)

	// Simulate a workspace directory deleted outside wtf.
	require.NoError(t, os.RemoveAll(wsPath))

	cleanDryRun = true
	t.Cleanup(func() { cleanDryRun = false })

	cmd, stdout, _ := newTestCmd("")
	require.NoError(t, runClean(cmd, mgr, nil))
	assert.Contains(t, stdout.String(), "prunable")
	assert.Contains(t, stdout.String(), "feat")
}

func TestJJ_PortIsStableAcrossWorkspaces(t *testing.T) {
	mgr, root := jjTestManager(t)

	alloc, err := portAllocator(mgr, root)
	require.NoError(t, err)

	first, err := alloc.Allocate("feat")
	require.NoError(t, err)

	// A second allocation for the same name must return the same port, and the
	// store lives with the repo so other workspaces see it too.
	again, err := alloc.Allocate("feat")
	require.NoError(t, err)
	assert.Equal(t, first, again)

	other, err := alloc.Allocate("feat-2")
	require.NoError(t, err)
	assert.NotEqual(t, first, other)
}

func TestJJ_GlobalListingTagsBackends(t *testing.T) {
	resetVCSState(t)

	jjRoot := initCLITestJJRepo(t)
	require.NoError(t, config.SetVCSPref(jjRoot, vcs.KindJJ))
	gitRoot := initCLITestRepo(t)
	// Each fixture deliberately gets a fresh WTF_HOME; re-establish the JJ
	// preference in the home owned by the last fixture before registry writes.
	require.NoError(t, config.SetVCSPref(jjRoot, vcs.KindJJ))

	require.NoError(t, config.Add(jjRoot))
	require.NoError(t, config.Add(gitRoot))

	cmd, _, _ := newTestCmd("")
	groups, err := collectGlobalStrict(cmd, []string{jjRoot, gitRoot})
	require.NoError(t, err)

	byKind := map[vcs.Kind]int{}
	for _, g := range groups {
		byKind[g.kind()]++
	}
	assert.Equal(t, 1, byKind[vcs.KindJJ], "the jj repo contributes its jj workspaces")
	assert.Equal(t, 1, byKind[vcs.KindGit], "the git repo contributes its git worktrees")
}

func TestJJ_GlobalListingSplitsUndecidedColocatedRepo(t *testing.T) {
	resetVCSState(t)

	// A colocated repo with no recorded preference genuinely holds both kinds of
	// checkout, so a global listing must show both rather than pick one silently.
	root := initCLITestJJRepo(t)
	require.NoError(t, config.Add(root))

	cmd, _, _ := newTestCmd("")
	groups, err := collectGlobalStrict(cmd, []string{root})
	require.NoError(t, err)
	require.Len(t, groups, 2)
}

func TestJJ_GlobalJSONCarriesVCS(t *testing.T) {
	resetVCSState(t)

	root := initCLITestJJRepo(t)
	require.NoError(t, config.SetVCSPref(root, vcs.KindJJ))

	jsonOutput = true
	t.Cleanup(func() { jsonOutput = false })

	cmd, stdout, _ := newTestCmd("")
	require.NoError(t, runLsGlobalJSON(cmd, []string{root}))

	var entries []struct {
		Repo      string `json:"repo"`
		VCS       string `json:"vcs"`
		Worktrees []struct {
			Branch   string `json:"branch"`
			VCS      string `json:"vcs"`
			ChangeID string `json:"change_id"`
		} `json:"worktrees"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "jj", entries[0].VCS)
	require.NotEmpty(t, entries[0].Worktrees)
	assert.Equal(t, "jj", entries[0].Worktrees[0].VCS)
	assert.NotEmpty(t, entries[0].Worktrees[0].ChangeID, "jj entries carry a change id")
}

func TestJJ_FindGlobalMatchesAreLabeledByBackend(t *testing.T) {
	resetVCSState(t)

	root := initCLITestJJRepo(t)
	require.NoError(t, config.SetVCSPref(root, vcs.KindJJ))

	mgr := newManager(vcs.KindJJ)
	_, err := mgr.Add(root, "shared-name", "main")
	require.NoError(t, err)

	cmd, _, _ := newTestCmd("")
	matches, err := findGlobalStrict(cmd, []string{root}, "shared-name")
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Contains(t, matches[0].label(), "jj")
	assert.Contains(t, matches[0].label(), filepath.Base(root))
}

func TestJJ_RmInteractiveGlobalUsesRowBackend(t *testing.T) {
	resetVCSState(t)

	root := initCLITestJJRepo(t)
	require.NoError(t, config.SetVCSPref(root, vcs.KindJJ))

	mgr := newManager(vcs.KindJJ)
	wsPath, err := mgr.Add(root, "doomed", "main")
	require.NoError(t, err)

	prevTTY := stdinIsTTY
	stdinIsTTY = func() bool { return true }
	t.Cleanup(func() { stdinIsTTY = prevTTY })

	// The picker selects the row; the row carries its own backend, so removal
	// needs no prompting about git vs jj.
	prevPicker := runPickerFunc
	var offered []ui.PickerItem
	runPickerFunc = func(items []ui.PickerItem, _ bool) (ui.PickerResult, error) {
		offered = items
		for _, it := range items {
			if it.Branch == "doomed" {
				return ui.PickerResult{Items: []ui.PickerItem{it}}, nil
			}
		}
		return ui.PickerResult{Quit: true}, nil
	}
	t.Cleanup(func() { runPickerFunc = prevPicker })

	cmd, stdout, _ := newTestCmd("")
	require.NoError(t, runRmInteractiveGlobal(cmd))

	require.NotEmpty(t, offered)
	assert.Equal(t, "jj", offered[0].VCS, "picker rows must be tagged with their backend")
	assert.Contains(t, stdout.String(), "Removed workspace for")
	assert.NoDirExists(t, wsPath)
}

func TestJJ_RmInteractiveGlobalExcludesMainAndQuits(t *testing.T) {
	resetVCSState(t)

	root := initCLITestJJRepo(t)
	require.NoError(t, config.SetVCSPref(root, vcs.KindJJ))

	prevTTY := stdinIsTTY
	stdinIsTTY = func() bool { return true }
	t.Cleanup(func() { stdinIsTTY = prevTTY })

	prevPicker := runPickerFunc
	var offered []ui.PickerItem
	runPickerFunc = func(items []ui.PickerItem, _ bool) (ui.PickerResult, error) {
		offered = items
		return ui.PickerResult{Quit: true}, nil
	}
	t.Cleanup(func() { runPickerFunc = prevPicker })

	cmd, _, stderr := newTestCmd("")
	require.NoError(t, runRmInteractiveGlobal(cmd))

	// Only the main workspace exists, and it is never removable.
	assert.Empty(t, offered)
	assert.Contains(t, stderr.String(), "No removable worktrees")
}

func TestJJ_RmInteractiveGlobalRequiresTTY(t *testing.T) {
	resetVCSState(t)

	prev := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	t.Cleanup(func() { stdinIsTTY = prev })

	cmd, _, _ := newTestCmd("")
	err := runRmInteractiveGlobal(cmd)
	assert.ErrorContains(t, err, "specify at least one branch")
}

func TestWarnOtherBackend(t *testing.T) {
	resetVCSState(t)
	root := initCLITestJJRepo(t)

	// Create a jj workspace, then look at the repo through the git backend: the
	// jj-side checkout must be mentioned rather than silently omitted.
	_, err := newManager(vcs.KindJJ).Add(root, "feat", "main")
	require.NoError(t, err)

	cmd, _, stderr := newTestCmd("")
	warnOtherBackend(cmd, newManager(vcs.KindGit), root)

	out := stderr.String()
	assert.Contains(t, out, "1 jj workspace also exists here")
	assert.Contains(t, out, "wtf sw --vcs jj")
}

func TestWarnOtherBackend_SilentWhenNothingExtra(t *testing.T) {
	resetVCSState(t)
	root := initCLITestJJRepo(t)

	// Only the shared primary checkout exists on the jj side, which is not news.
	cmd, _, stderr := newTestCmd("")
	warnOtherBackend(cmd, newManager(vcs.KindGit), root)
	assert.Empty(t, stderr.String())

	// A git-only repo has no other backend to mention at all.
	gitRoot := initCLITestRepo(t)
	cmd2, _, stderr2 := newTestCmd("")
	warnOtherBackend(cmd2, newManager(vcs.KindGit), gitRoot)
	assert.Empty(t, stderr2.String())
}

func TestHintOtherBackendMatch(t *testing.T) {
	resetVCSState(t)
	root := initCLITestJJRepo(t)

	_, err := newManager(vcs.KindJJ).Add(root, "only-in-jj", "main")
	require.NoError(t, err)

	// Missing under git but present under jj — say so instead of "not found".
	cmd, _, stderr := newTestCmd("")
	hintOtherBackendMatch(cmd, newManager(vcs.KindGit), root, "only-in-jj")
	assert.Contains(t, stderr.String(), "exists as a jj workspace here")
	assert.Contains(t, stderr.String(), "wtf sw --vcs jj only-in-jj")

	// A name absent from both backends produces no hint.
	cmd2, _, stderr2 := newTestCmd("")
	hintOtherBackendMatch(cmd2, newManager(vcs.KindGit), root, "nowhere-at-all")
	assert.Empty(t, stderr2.String())
}

func TestCollectGlobalSkipsUnusableRepos(t *testing.T) {
	resetVCSState(t)

	good := initCLITestRepo(t)
	bad := t.TempDir() // exists, but is not a repo

	cmd, _, stderr := newTestCmd("")
	groups, err := collectGlobalStrict(cmd, []string{good, bad})
	require.NoError(t, err)

	require.Len(t, groups, 1)
	assert.Equal(t, good, groups[0].repo)
	assert.Contains(t, stderr.String(), "Could not determine the backend")
}

func TestLoadGlobalReposErrorsWhenEmpty(t *testing.T) {
	resetVCSState(t)

	_, err := loadGlobalRepos()
	assert.ErrorContains(t, err, "no registered repos")
}

func TestPluralizeAndRefHeader(t *testing.T) {
	assert.Equal(t, "workspace", pluralize("workspace", 1))
	assert.Equal(t, "workspaces", pluralize("workspace", 2))
	assert.Equal(t, "workspaces", pluralize("workspace", 0))

	assert.Equal(t, "WORKSPACE", refHeader(vcs.KindJJ))
	assert.Equal(t, "BRANCH", refHeader(vcs.KindGit))
}

func TestPickerKindLabel(t *testing.T) {
	resetVCSState(t)

	// A colocated repo can hold both kinds, so picker rows earn a label.
	colo := initCLITestJJRepo(t)
	assert.Equal(t, vcs.KindJJ, pickerKindLabel(newManager(vcs.KindJJ), colo))
	assert.Equal(t, vcs.KindGit, pickerKindLabel(newManager(vcs.KindGit), colo))

	// A plain git repo does not: every row comes from the same place, so "(git)"
	// on each line is clutter for users who never touch jj.
	gitOnly := initCLITestRepo(t)
	assert.Equal(t, vcs.Kind(""), pickerKindLabel(newManager(vcs.KindGit), gitOnly))

	assert.Equal(t, vcs.Kind(""), pickerKindLabel(newManager(vcs.KindGit), t.TempDir()))
}

func TestLocalPickerIsUnlabeledInSingleBackendRepo(t *testing.T) {
	resetVCSState(t)
	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := newManager(vcs.KindGit)
	_, err := wm.Add(dir, "feat", "main")
	require.NoError(t, err)

	prevTTY := stdinIsTTY
	stdinIsTTY = func() bool { return true }
	t.Cleanup(func() { stdinIsTTY = prevTTY })

	prevPicker := runPickerFunc
	var offered []ui.PickerItem
	runPickerFunc = func(items []ui.PickerItem, _ bool) (ui.PickerResult, error) {
		offered = items
		return ui.PickerResult{Quit: true}, nil
	}
	t.Cleanup(func() { runPickerFunc = prevPicker })

	cmd, _, _ := newTestCmd("")
	require.NoError(t, runLs(cmd, wm))

	require.NotEmpty(t, offered)
	for _, it := range offered {
		assert.Empty(t, it.VCS, "a git-only repo must not tag every picker row")
	}
}

func TestGlobalPickerIsAlwaysLabeled(t *testing.T) {
	resetVCSState(t)
	root := initCLITestJJRepo(t)
	require.NoError(t, config.SetVCSPref(root, vcs.KindJJ))

	cmd, _, _ := newTestCmd("")
	groups, err := collectGlobalStrict(cmd, []string{root})
	require.NoError(t, err)
	items, _ := globalPickerItems(groups, func(_ globalGroup, _ vcs.Worktree) bool { return true })

	require.NotEmpty(t, items)
	for _, it := range items {
		assert.Equal(t, "jj", it.VCS, "global rows span repos, so they always carry a backend")
	}
}
