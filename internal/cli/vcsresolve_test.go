package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/jj"
	"github.com/AndrewPBerg/wtf/internal/vcs"
)

// requireJJ skips when the jj CLI is unavailable.
func requireJJ(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("jj"); err != nil {
		t.Skip("jj not installed — skipping jj CLI test")
	}
}

// initCLITestJJRepo creates a colocated jj repo with one commit, a `main`
// bookmark, and a gitignored .env, returning its root.
func initCLITestJJRepo(t *testing.T) string {
	t.Helper()
	requireJJ(t)

	base := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}
	root := filepath.Join(base, "myrepo")
	require.NoError(t, os.MkdirAll(root, 0o755))

	run := func(args ...string) {
		t.Helper()
		_, err := (&jj.RealExecutor{}).Run(root, args...)
		require.NoError(t, err, "jj %v", args)
	}

	run("git", "init", "--colocate")
	run("config", "set", "--repo", "user.name", "wtf test")
	run("config", "set", "--repo", "user.email", "wtf@example.com")

	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".env\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("hi\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1\n"), 0o644))
	run("commit", "-m", "init")
	run("bookmark", "create", "main", "-r", "@-")

	return root
}

// newTestCmd returns a cobra command with buffered output and the given stdin.
func newTestCmd(stdin string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	cmd := &cobra.Command{}
	stdout, stderr := new(bytes.Buffer), new(bytes.Buffer)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetIn(strings.NewReader(stdin))
	return cmd, stdout, stderr
}

// resetVCSState clears the flag, env, and registry between cases so each test
// starts from a clean dispatch state.
func resetVCSState(t *testing.T) {
	t.Helper()
	prev := vcsFlag
	vcsFlag = ""
	t.Cleanup(func() { vcsFlag = prev })
	t.Setenv("WTF_VCS", "")
	t.Setenv("WTF_HOME", t.TempDir())
}

func TestChooseKindQuiet(t *testing.T) {
	gitOnly := vcs.Detection{Root: "/code/g", Kinds: []vcs.Kind{vcs.KindGit}}
	jjOnly := vcs.Detection{Root: "/code/j", Kinds: []vcs.Kind{vcs.KindJJ}}
	both := vcs.Detection{Root: "/code/b", Kinds: []vcs.Kind{vcs.KindGit, vcs.KindJJ}}

	tests := []struct {
		name          string
		det           vcs.Detection
		flag          string
		env           string
		pref          vcs.Kind
		wantKind      vcs.Kind
		wantAmbiguous bool
		wantErr       bool
	}{
		{name: "git only", det: gitOnly, wantKind: vcs.KindGit},
		{name: "jj only", det: jjOnly, wantKind: vcs.KindJJ},
		{
			name: "colocated with nothing recorded is ambiguous but defaults to git",
			det:  both, wantKind: vcs.KindGit, wantAmbiguous: true,
		},
		{name: "flag wins over everything", det: both, flag: "jj", env: "git", pref: vcs.KindGit, wantKind: vcs.KindJJ},
		{name: "env wins over saved preference", det: both, env: "jj", pref: vcs.KindGit, wantKind: vcs.KindJJ},
		{name: "saved preference is used", det: both, pref: vcs.KindJJ, wantKind: vcs.KindJJ},
		{name: "invalid flag errors", det: both, flag: "hg", wantErr: true},
		{name: "invalid env errors", det: both, env: "svn", wantErr: true},
		{
			name: "flag naming a backend the repo does not have errors",
			det:  gitOnly, flag: "jj", wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetVCSState(t)
			vcsFlag = tt.flag
			t.Setenv("WTF_VCS", tt.env)
			if tt.pref != "" {
				require.NoError(t, config.SetVCSPref(tt.det.Root, tt.pref))
			}

			kind, ambiguous, err := chooseKindQuiet(tt.det)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantKind, kind)
			assert.Equal(t, tt.wantAmbiguous, ambiguous)
		})
	}
}

func TestPromptVCS(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    vcs.Kind
		wantErr bool
	}{
		{name: "1 selects jj", input: "1\n", want: vcs.KindJJ},
		{name: "2 selects git", input: "2\n", want: vcs.KindGit},
		{name: "name jj", input: "jj\n", want: vcs.KindJJ},
		{name: "name git", input: "git\n", want: vcs.KindGit},
		{name: "case insensitive", input: "JJ\n", want: vcs.KindJJ},
		{name: "surrounding whitespace", input: "  2  \n", want: vcs.KindGit},
		{name: "no default on empty input", input: "\n", wantErr: true},
		{name: "invalid choice", input: "maybe\n", wantErr: true},
		{name: "closed stdin", input: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, _, stderr := newTestCmd(tt.input)

			got, err := promptVCS(cmd, "/code/myrepo")
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			// The prompt names the repo and both options.
			assert.Contains(t, stderr.String(), "myrepo")
			assert.Contains(t, stderr.String(), "jj")
			assert.Contains(t, stderr.String(), "git")
		})
	}
}

func TestResolveManager_ColocatedPromptsAndPersists(t *testing.T) {
	resetVCSState(t)
	root := initCLITestJJRepo(t)
	t.Chdir(root)

	// Pretend a terminal is attached so the ambiguous case prompts.
	prev := stdinIsTTY
	stdinIsTTY = func() bool { return true }
	t.Cleanup(func() { stdinIsTTY = prev })

	cmd, _, _ := newTestCmd("1\n")
	mgr, err := resolveManager(cmd)
	require.NoError(t, err)
	assert.Equal(t, vcs.KindJJ, mgr.Kind())

	// The answer is saved, so a second command must not prompt: an empty stdin
	// would fail if it did.
	saved, ok := config.VCSPref(root)
	require.True(t, ok, "choice should be persisted")
	assert.Equal(t, vcs.KindJJ, saved)

	cmd2, _, _ := newTestCmd("")
	mgr2, err := resolveManager(cmd2)
	require.NoError(t, err)
	assert.Equal(t, vcs.KindJJ, mgr2.Kind())
}

func TestResolveManager_ColocatedNonTTYPrefersJJWhenJJOwnsWorkingCopy(t *testing.T) {
	resetVCSState(t)
	root := initCLITestJJRepo(t)
	t.Chdir(root)

	prev := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	t.Cleanup(func() { stdinIsTTY = prev })

	cmd, _, stderr := newTestCmd("")
	mgr, err := resolveManager(cmd)
	require.NoError(t, err)

	// A fresh colocated repo has no secondary checkouts to learn from, but jj
	// leaves git's HEAD detached when it drives the working copy. Without that
	// signal a `jj git clone` would be handed git worktrees in CI.
	assert.Equal(t, vcs.KindJJ, mgr.Kind())
	assert.Empty(t, stderr.String(), "an inferred choice needs no warning")

	// Inference is not a user choice, so it must not be persisted.
	_, ok := config.VCSPref(root)
	assert.False(t, ok)
}

func TestResolveManager_ColocatedNonTTYFallsBackToGitWhenGitDrives(t *testing.T) {
	resetVCSState(t)
	root := initCLITestGitDrivenColocatedRepo(t)
	t.Chdir(root)

	prev := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	t.Cleanup(func() { stdinIsTTY = prev })

	cmd, _, stderr := newTestCmd("")
	mgr, err := resolveManager(cmd)
	require.NoError(t, err)

	// git owns HEAD here, so there is no evidence for jj. git is the documented
	// fallback, announced rather than chosen silently.
	assert.Equal(t, vcs.KindGit, mgr.Kind())
	assert.Contains(t, stderr.String(), "both a git and a jj repo")
	assert.Contains(t, stderr.String(), "WTF_VCS=jj")

	_, ok := config.VCSPref(root)
	assert.False(t, ok, "the non-TTY fallback is not a user choice")
}

func TestResolveManager_ColocatedNonTTYInfersFromExistingCheckouts(t *testing.T) {
	// Colocation is jj's default layout, so a jj user's repo looks "ambiguous" by
	// markers alone. Existing checkouts are better evidence than a coin flip.
	t.Run("existing jj workspaces select jj", func(t *testing.T) {
		resetVCSState(t)
		root := initCLITestJJRepo(t)
		_, err := newManager(vcs.KindJJ).Add(root, "feat", "main")
		require.NoError(t, err)
		t.Chdir(root)

		prev := stdinIsTTY
		stdinIsTTY = func() bool { return false }
		t.Cleanup(func() { stdinIsTTY = prev })

		cmd, _, stderr := newTestCmd("")
		mgr, err := resolveManager(cmd)
		require.NoError(t, err)
		assert.Equal(t, vcs.KindJJ, mgr.Kind())
		assert.Empty(t, stderr.String(), "an inferred choice needs no warning")
	})

	t.Run("existing git worktrees select git", func(t *testing.T) {
		resetVCSState(t)
		root := initCLITestJJRepo(t)
		// A git worktree already present means wtf was being used with git here,
		// so inference must preserve that rather than switch the user to jj.
		_, err := newManager(vcs.KindGit).Add(root, "legacy", "main")
		require.NoError(t, err)
		t.Chdir(root)

		prev := stdinIsTTY
		stdinIsTTY = func() bool { return false }
		t.Cleanup(func() { stdinIsTTY = prev })

		cmd, _, stderr := newTestCmd("")
		mgr, err := resolveManager(cmd)
		require.NoError(t, err)
		assert.Equal(t, vcs.KindGit, mgr.Kind())
		assert.Empty(t, stderr.String())
	})
}

func TestInferKindFromExisting(t *testing.T) {
	t.Run("both kinds present is undecidable", func(t *testing.T) {
		resetVCSState(t)
		root := initCLITestJJRepo(t)

		_, err := newManager(vcs.KindJJ).Add(root, "ws", "main")
		require.NoError(t, err)
		_, err = newManager(vcs.KindGit).Add(root, "wt", "main")
		require.NoError(t, err)

		_, ok := inferKindFromExisting(root)
		assert.False(t, ok, "with evidence for both, wtf must not guess")
	})

	t.Run("git-driven colocated repo with no checkouts is undecidable", func(t *testing.T) {
		resetVCSState(t)
		_, ok := inferKindFromExisting(initCLITestGitDrivenColocatedRepo(t))
		assert.False(t, ok)
	})

	t.Run("not a repo", func(t *testing.T) {
		resetVCSState(t)
		// A directory with no git repo must not be mistaken for a detached HEAD.
		_, ok := inferKindFromExisting(t.TempDir())
		assert.False(t, ok)
	})

	t.Run("git repo with no commits yet", func(t *testing.T) {
		resetVCSState(t)
		dir := t.TempDir()
		_, err := (&git.RealExecutor{}).Run(dir, "init", "-b", "main")
		require.NoError(t, err)
		// HEAD is unborn, so its shape says nothing about who drives the repo.
		_, ok := inferKindFromExisting(dir)
		assert.False(t, ok)
	})
}

func TestResolveManager_NotARepo(t *testing.T) {
	resetVCSState(t)
	t.Chdir(t.TempDir())

	cmd, _, _ := newTestCmd("")
	_, err := resolveManager(cmd)
	assert.ErrorIs(t, err, ErrNotARepo)
}

func TestRepoDirFor_InsideJJWorkspace(t *testing.T) {
	resetVCSState(t)
	root := initCLITestJJRepo(t)

	mgr := newManager(vcs.KindJJ)
	wsPath, err := mgr.Add(root, "feat", "main")
	require.NoError(t, err)

	// A secondary jj workspace has no .git, so git discovery cannot work here.
	// This is the case that made wtf unusable inside jj workspaces.
	require.NoFileExists(t, filepath.Join(wsPath, ".git"))
	t.Chdir(wsPath)

	got, err := repoDirFor(mgr)
	require.NoError(t, err)
	assert.Equal(t, wsPath, got)
}

func TestGetRepoDir_InJJOnlyRepo(t *testing.T) {
	resetVCSState(t)
	root := initCLITestJJRepo(t)

	// Remove the git side so the repo is unambiguously jj.
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".git")))
	t.Chdir(root)

	got, err := getRepoDir()
	require.NoError(t, err)
	assert.Equal(t, root, got)
}

func TestManagersForRepo(t *testing.T) {
	resetVCSState(t)
	root := initCLITestJJRepo(t)

	// Colocated with no preference: both backends are reported so a global
	// listing can show everything that exists.
	mgrs := managersForRepo(root)
	require.Len(t, mgrs, 2)
	kinds := []vcs.Kind{mgrs[0].Kind(), mgrs[1].Kind()}
	assert.Contains(t, kinds, vcs.KindGit)
	assert.Contains(t, kinds, vcs.KindJJ)

	// With a preference recorded, only that backend is used.
	require.NoError(t, config.SetVCSPref(root, vcs.KindJJ))
	mgrs = managersForRepo(root)
	require.Len(t, mgrs, 1)
	assert.Equal(t, vcs.KindJJ, mgrs[0].Kind())

	// A directory that is not a repo yields nothing.
	assert.Empty(t, managersForRepo(t.TempDir()))
}

func TestValidateRef_PerBackend(t *testing.T) {
	resetVCSState(t)

	gitMgr := newManager(vcs.KindGit)
	jjMgr := newManager(vcs.KindJJ)

	// Both accept ordinary names, including slashes.
	assert.NoError(t, validateRef(gitMgr, "feature/auth"))
	assert.NoError(t, validateRef(jjMgr, "feature/auth"))

	// Both reject empty names.
	assert.Error(t, validateRef(gitMgr, ""))
	assert.Error(t, validateRef(jjMgr, ""))

	// jj rejects whitespace in workspace names.
	assert.ErrorIs(t, validateRef(jjMgr, "feat auth"), vcs.ErrInvalidRef)
}

func TestPortAllocatorUsesBackendStateDir(t *testing.T) {
	resetVCSState(t)
	root := initCLITestJJRepo(t)

	// jj keeps state under .jj/repo rather than .git, so every workspace of the
	// repo agrees on one ports file.
	jjMgr := newManager(vcs.KindJJ)
	alloc, err := portAllocator(jjMgr, root)
	require.NoError(t, err)
	require.NotNil(t, alloc)

	p, err := alloc.Allocate("feat")
	require.NoError(t, err)
	assert.Positive(t, p)
	assert.FileExists(t, filepath.Join(root, ".jj", "repo", "wtf", "ports.json"))
}

// initCLITestGitDrivenColocatedRepo builds a repo that is both git and jj but whose
// working copy git still owns, i.e. HEAD is on a branch rather than detached. This
// is the "someone added jj to a git repo" shape, as opposed to `jj git clone`.
func initCLITestGitDrivenColocatedRepo(t *testing.T) string {
	t.Helper()
	root := initCLITestJJRepo(t)

	// jj detaches HEAD when it updates the working copy; reattaching it makes git
	// the apparent owner again.
	_, err := (&git.RealExecutor{}).Run(root, "checkout", "-B", "main")
	require.NoError(t, err)

	return root
}
