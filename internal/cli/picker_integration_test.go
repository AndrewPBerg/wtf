package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/AndrewPBerg/wtf/internal/ui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AndrewPBerg/wtf/internal/vcs"
)

// --- helpers ---

// withPickerStub overrides runPickerFunc for the test's duration.
// The stub records the items it was called with and returns the given result.
func withPickerStub(t *testing.T, result ui.PickerResult) *[]ui.PickerItem {
	t.Helper()
	var captured []ui.PickerItem
	orig := runPickerFunc
	runPickerFunc = func(items []ui.PickerItem, _ bool) (ui.PickerResult, error) {
		captured = items
		return result, nil
	}
	t.Cleanup(func() { runPickerFunc = orig })
	return &captured
}

func resetLsFlags(t *testing.T) {
	t.Helper()
	origGlobal, origPRs, origJSON := lsGlobal, lsPRs, jsonOutput
	lsGlobal = false
	lsPRs = false
	jsonOutput = false
	t.Cleanup(func() {
		lsGlobal = origGlobal
		lsPRs = origPRs
		jsonOutput = origJSON
	})
}

// --- Shell wrapper integration ---

func TestShellWrapper_ContainsSwg(t *testing.T) {
	for _, shell := range []setup.Shell{setup.Bash, setup.Zsh, setup.Fish} {
		t.Run(string(shell), func(t *testing.T) {
			out := setup.Render(shell, setup.DefaultFuncs(), nil)
			switch shell {
			case setup.Fish:
				assert.Contains(t, out, "sw swg news", "fish wrapper must intercept swg")
			default:
				assert.Contains(t, out, "sw|swg|news", "bash/zsh wrapper must intercept swg")
			}
		})
	}
}

func TestShellWrapper_EmptyPathGuard(t *testing.T) {
	for _, shell := range []setup.Shell{setup.Bash, setup.Zsh, setup.Fish} {
		t.Run(string(shell), func(t *testing.T) {
			out := setup.Render(shell, setup.DefaultFuncs(), nil)
			switch shell {
			case setup.Fish:
				assert.Contains(t, out, `test -z "$_p"`, "fish must guard empty path (cancelled picker)")
			default:
				assert.Contains(t, out, `[ -z "$_p" ]`, "bash/zsh must guard empty path (cancelled picker)")
			}
		})
	}
}

// --- Local interactive picker ---

func TestRunLs_Interactive_PrintsPathToStdout(t *testing.T) {
	resetLsFlags(t)
	withTTY(t)

	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	wtPath, err := wm.Add(dir, "feature-pick", "main")
	require.NoError(t, err)

	pickerResult := ui.PickerResult{
		Items: []ui.PickerItem{{Branch: "feature-pick", Path: wtPath}},
	}
	withPickerStub(t, pickerResult)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runLs(cmd, wm)
	require.NoError(t, err)

	// Path must go to stdout (shell wrapper captures this with $()).
	assert.Contains(t, stdout.String(), wtPath)
	// Status message on stderr.
	assert.Contains(t, stderr.String(), "Switched to")
}

func TestRunLs_Interactive_CancelledPicker(t *testing.T) {
	resetLsFlags(t)
	withTTY(t)

	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	_, err := wm.Add(dir, "feature-cancel", "main")
	require.NoError(t, err)

	withPickerStub(t, ui.PickerResult{Quit: true})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runLs(cmd, wm)
	require.NoError(t, err)

	// No output when user cancels — shell wrapper sees empty string and returns.
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRunLs_NonTTY_FallsBackToTable(t *testing.T) {
	resetLsFlags(t)

	// Force non-TTY (default in tests, but be explicit).
	orig := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	t.Cleanup(func() { stdinIsTTY = orig })

	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})

	stdout := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(stdout)

	err := runLs(cmd, wm)
	require.NoError(t, err)

	// Static table output — not the picker.
	output := stdout.String()
	assert.Contains(t, output, "BRANCH")
	assert.Contains(t, output, "PATH")
	assert.Contains(t, output, "HEAD")
}

// --- Global interactive picker ---

func TestRunLsGlobal_Interactive_PrintsPathToStdout(t *testing.T) {
	resetLsFlags(t)
	lsGlobal = true
	withTTY(t)

	repo1 := initCLITestRepo(t)
	repo2 := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo1, repo2})

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	wtPath, err := wm.Add(repo2, "feature-global-pick", "main")
	require.NoError(t, err)

	pickerResult := ui.PickerResult{
		Items: []ui.PickerItem{{
			Branch: "feature-global-pick",
			Path:   wtPath,
			Repo:   filepath.Base(repo2),
		}},
	}
	captured := withPickerStub(t, pickerResult)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runLs(cmd, wm)
	require.NoError(t, err)

	// Path to stdout, status to stderr.
	assert.Contains(t, stdout.String(), wtPath)
	assert.Contains(t, stderr.String(), "Switched to")

	// Picker must receive items from BOTH repos.
	assert.Greater(t, len(*captured), 1, "picker should receive worktrees from multiple repos")
}

func TestRunLsGlobal_Interactive_CancelledPicker(t *testing.T) {
	resetLsFlags(t)
	lsGlobal = true
	withTTY(t)

	repo := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo})

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	withPickerStub(t, ui.PickerResult{Quit: true})

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err := runLs(cmd, wm)
	require.NoError(t, err)

	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestRunLsGlobal_NonTTY_FallsBackToTable(t *testing.T) {
	resetLsFlags(t)
	lsGlobal = true

	orig := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	t.Cleanup(func() { stdinIsTTY = orig })

	repo := initCLITestRepo(t)
	setupGlobalRegistry(t, []string{repo})

	wm := git.NewWorktreeManager(&git.RealExecutor{})

	stdout := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(stdout)

	err := runLs(cmd, wm)
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "BRANCH")
	assert.Contains(t, output, "main *")
}

// --- Picker item filtering ---

func TestWorktreesToPickerItems_FiltersDetachedAndBare(t *testing.T) {
	wts := []git.Worktree{
		{Branch: "main", Path: "/repo", Head: "abc1234", IsMain: true},
		{Branch: "feature", Path: "/repo--feature", Head: "def5678"},
		{Branch: "", Path: "/repo--detached", Head: "111", IsDetached: true},
		{Branch: "", Path: "/repo--bare", IsBare: true},
		{Branch: "", Path: "/repo--empty-branch", Head: "222"}, // empty branch name
	}

	items := worktreesToPickerItems(wts, "", vcs.KindGit)
	assert.Len(t, items, 2) // only main and feature
	assert.Equal(t, "main", items[0].Branch)
	assert.Equal(t, "feature", items[1].Branch)
}

func TestWorktreesToPickerItems_GlobalSetsRepo(t *testing.T) {
	wts := []git.Worktree{
		{Branch: "main", Path: "/repo", Head: "abc1234", IsMain: true},
	}

	items := worktreesToPickerItems(wts, "my-repo", vcs.KindGit)
	require.Len(t, items, 1)
	assert.Equal(t, "my-repo", items[0].Repo)
}

func TestWorktreesToPickerItems_EmptyInput(t *testing.T) {
	items := worktreesToPickerItems(nil, "", vcs.KindGit)
	assert.Empty(t, items)
}

// --- Interactive already-on-branch detection ---

func TestRunLs_Interactive_AlreadyOnBranch(t *testing.T) {
	resetLsFlags(t)
	withTTY(t)

	dir := initCLITestRepo(t)
	wm := git.NewWorktreeManager(&git.RealExecutor{})
	wtPath, err := wm.Add(dir, "feature-here", "main")
	require.NoError(t, err)

	// cd into the target worktree
	t.Chdir(wtPath)

	pickerResult := ui.PickerResult{
		Items: []ui.PickerItem{{Branch: "feature-here", Path: wtPath}},
	}
	withPickerStub(t, pickerResult)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := swCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runLs(cmd, wm)
	require.NoError(t, err)

	// Should still print path (shell wrapper needs it).
	assert.Contains(t, stdout.String(), wtPath)
	// Should warn user they're already there.
	assert.Contains(t, stderr.String(), "wtf?")
	assert.Contains(t, stderr.String(), "already on")
}

// --- rm interactive picker ---

func TestRunRmInteractive_NoArgs(t *testing.T) {
	resetRmFlags(t)
	withTTY(t)

	dir := initCLITestRepo(t)
	t.Chdir(dir)

	wm := git.NewWorktreeManager(&git.RealExecutor{})
	_, err := wm.Add(dir, "feature-rm-pick", "main")
	require.NoError(t, err)

	// List worktrees to get the real path for the picker result.
	wts, err := wm.List(dir)
	require.NoError(t, err)
	var wtPath string
	for _, wt := range wts {
		if wt.Branch == "feature-rm-pick" {
			wtPath = wt.Path
		}
	}
	require.NotEmpty(t, wtPath)

	pickerResult := ui.PickerResult{
		Items: []ui.PickerItem{{Branch: "feature-rm-pick", Path: wtPath}},
	}
	withPickerStub(t, pickerResult)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cmd := rmCmd
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)

	err = runRmInteractive(cmd, wm)
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "Removed worktree for")
	assert.Contains(t, stdout.String(), "feature-rm-pick")

	// Worktree should actually be removed.
	wts, err = wm.List(dir)
	require.NoError(t, err)
	for _, wt := range wts {
		assert.NotEqual(t, "feature-rm-pick", wt.Branch)
	}
}

func TestRunRmInteractive_NonTTY_RequiresArgs(t *testing.T) {
	resetRmFlags(t)

	orig := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	t.Cleanup(func() { stdinIsTTY = orig })

	dir := initCLITestRepo(t)
	t.Chdir(dir)

	cmd := rmCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))

	// The RunE handler checks stdinIsTTY and returns error when no args + no TTY.
	err := rmCmd.RunE(cmd, nil)
	assert.Error(t, err)
}

// --- removablePickerItems filtering ---

func TestRemovablePickerItems_ExcludesMainAndCurrent(t *testing.T) {
	dir := initCLITestRepo(t)
	wm := git.NewWorktreeManager(&git.RealExecutor{})
	wtPath, err := wm.Add(dir, "feature-removable", "main")
	require.NoError(t, err)
	_, err = wm.Add(dir, "feature-other", "main")
	require.NoError(t, err)

	wts, err := wm.List(dir)
	require.NoError(t, err)

	// Pretend we're inside feature-removable (should be excluded as current).
	items := removablePickerItems(wts, wtPath, "", vcs.KindGit)

	for _, item := range items {
		assert.NotEqual(t, "main", item.Branch, "main worktree must not appear in rm picker")
		// Current worktree (feature-removable) should be excluded.
		assert.NotEqual(t, wtPath, item.Path, "current worktree must not appear in rm picker")
	}
	// feature-other should be present.
	assert.Len(t, items, 1)
	assert.Equal(t, "feature-other", items[0].Branch)
}

// --- Symmetric TTY behavior: local vs global use the SAME check ---

func TestRunLs_TTYCheck_SymmetricLocalAndGlobal(t *testing.T) {
	// This test guards against the bug where local used stdinIsTTY
	// but global used isatty on stdout. Both must use stdinIsTTY so
	// the shell wrapper's $() capture doesn't break the picker.
	resetLsFlags(t)
	withTTY(t)

	repo := initCLITestRepo(t)
	t.Chdir(repo)
	setupGlobalRegistry(t, []string{repo})

	wm := git.NewWorktreeManager(&git.RealExecutor{})

	localCaptured := withPickerStub(t, ui.PickerResult{Quit: true})

	// Local path.
	lsGlobal = false
	cmd := swCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	err := runLs(cmd, wm)
	require.NoError(t, err)
	localHadItems := len(*localCaptured) > 0

	// Global path.
	lsGlobal = true
	globalCaptured := withPickerStub(t, ui.PickerResult{Quit: true})
	cmd = swCmd
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetErr(new(bytes.Buffer))
	err = runLs(cmd, wm)
	require.NoError(t, err)
	globalHadItems := len(*globalCaptured) > 0

	// Both must have reached the picker (stdinIsTTY was true).
	assert.True(t, localHadItems, "local picker was not invoked despite TTY stdin")
	assert.True(t, globalHadItems, "global picker was not invoked despite TTY stdin — likely still using stdout TTY check")
}
