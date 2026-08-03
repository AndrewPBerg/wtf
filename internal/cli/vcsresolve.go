package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/config"
	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/jj"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/spf13/cobra"
)

// vcsFlag is the value of the global --vcs flag: "git", "jj", or empty for
// auto-detection.
var vcsFlag string

// newManager builds the backend driver for a kind.
func newManager(kind vcs.Kind) vcs.Manager {
	if kind == vcs.KindJJ {
		return jj.NewWorkspaceManager(&jj.RealExecutor{})
	}
	return git.NewWorktreeManager(&git.RealExecutor{})
}

// resolveManager decides which backend to use for the repo the caller is in.
//
// Detection is unambiguous when only one of .git or .jj is present. A colocated
// repo has both — which is jj's own default layout, not a rarity — so the choice is
// resolved in priority order: the --vcs flag, then WTF_VCS, then a preference saved
// for this repo, then an interactive prompt whose answer is saved. With no terminal
// to ask on, the existing checkouts decide, and only a repo with none at all falls
// back to git.
func resolveManager(cmd *cobra.Command) (vcs.Manager, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	det, err := vcs.Detect(cwd)
	if err != nil {
		return nil, ErrNotARepo
	}

	kind, ambiguous, err := chooseKindQuiet(det)
	if err != nil {
		return nil, err
	}

	// Only a colocated repo with nothing recorded is genuinely ambiguous.
	if ambiguous {
		switch {
		case stdinIsTTY():
			kind, err = promptVCS(cmd, det.Root)
			if err != nil {
				return nil, err
			}
			// Remember the answer so the prompt appears once per repo rather
			// than once per command.
			if setErr := config.SetVCSPref(det.Root, kind); setErr != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
					"%s could not save your choice: %v\n", yellow("⚠"), setErr)
			}
		default:
			// No terminal to ask on. Rather than flip a coin, infer from the
			// checkouts the repo already has — see inferKindFromExisting.
			inferred, ok := inferKindFromExisting(det.Root)
			if ok {
				kind = inferred
				break
			}
			// Nothing to go on. Default to git, which is what a colocated repo
			// resolved to before jj support existed, and say so.
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
				"%s %s is both a git and a jj repo — defaulting to %s\n  %s pass %s or set %s\n",
				yellow("⚠"), cyan(filepath.Base(det.Root)), cyan("git"),
				dim("hint:"), cyan("--vcs jj"), cyan("WTF_VCS=jj"))
			kind = vcs.KindGit
		}
	}

	if !vcs.Available(kind) {
		return nil, fmt.Errorf("%s is required for this repo but was not found on PATH", kind.Label())
	}

	return newManager(kind), nil
}

// chooseKindQuiet resolves a Detection to a backend without prompting or
// printing. The second return value reports that the repo is colocated and
// nothing has been recorded, so the caller must decide how to disambiguate.
func chooseKindQuiet(det vcs.Detection) (vcs.Kind, bool, error) {
	// An explicit --vcs is honored even in a single-backend repo, so a wrong
	// value is reported rather than silently ignored.
	if vcsFlag != "" {
		kind, err := vcs.ParseKind(vcsFlag)
		if err != nil {
			return "", false, err
		}
		if !det.Has(kind) {
			return "", false, fmt.Errorf("--vcs %s was requested but %s is not a %s repo",
				kind.Label(), det.Root, kind.Label())
		}
		return kind, false, nil
	}

	if !det.Colocated() {
		return det.Kinds[0], false, nil
	}

	if env := strings.TrimSpace(os.Getenv("WTF_VCS")); env != "" {
		kind, err := vcs.ParseKind(env)
		if err != nil {
			return "", false, fmt.Errorf("WTF_VCS: %w", err)
		}
		return kind, false, nil
	}

	if saved, ok := config.VCSPref(det.Root); ok {
		return saved, false, nil
	}

	return vcs.KindGit, true, nil
}

// resolveManagerQuiet resolves the backend for the cwd without prompting. Used by
// helpers that run after a command has already resolved its backend, so the
// ambiguous case is settled by then.
func resolveManagerQuiet() (vcs.Manager, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("getting working directory: %w", err)
	}

	det, err := vcs.Detect(cwd)
	if err != nil {
		return nil, ErrNotARepo
	}

	kind, _, err := chooseKindQuiet(det)
	if err != nil {
		return nil, err
	}
	if !vcs.Available(kind) {
		return nil, fmt.Errorf("%s is required for this repo but was not found on PATH", kind.Label())
	}
	return newManager(kind), nil
}

// promptVCS asks which backend to use for a colocated repo.
func promptVCS(cmd *cobra.Command, root string) (vcs.Kind, error) {
	stderr := cmd.ErrOrStderr()

	_, _ = fmt.Fprintf(stderr, "\n%s %s is both a git and a jj repo — which should wtf use?\n",
		yellow("?"), cyan(filepath.Base(root)))
	_, _ = fmt.Fprintf(stderr, "  %s %s   %s\n",
		cyanBold("[1]"), cyan("jj"), dim("workspace  (jj manages the working copy here)"))
	_, _ = fmt.Fprintf(stderr, "  %s %s  %s\n",
		cyanBold("[2]"), cyan("git"), dim("worktree"))
	_, _ = fmt.Fprintf(stderr, "\nUse which? [1-2] %s ", dim("(saved for this repo)"))

	scanner := bufio.NewScanner(cmd.InOrStdin())
	if !scanner.Scan() {
		return "", fmt.Errorf("no choice made — pass --vcs git or --vcs jj")
	}

	switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
	case "1", "jj":
		return vcs.KindJJ, nil
	case "2", "git":
		return vcs.KindGit, nil
	default:
		return "", fmt.Errorf("invalid choice — pass --vcs git or --vcs jj")
	}
}

// repoDirFor returns the root of the checkout the caller is standing in, using
// the given backend's own discovery, and auto-registers the repo.
//
// The backend has to do the discovery: a secondary jj workspace contains no .git
// at all, so `git rev-parse --show-toplevel` fails outright there — which is why
// wtf did not work inside jj workspaces before.
func repoDirFor(mgr vcs.Manager) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getting working directory: %w", err)
	}

	var dir string
	switch mgr.Kind() {
	case vcs.KindJJ:
		dir, err = jj.WorkspaceRoot(cwd)
		if err != nil {
			return "", ErrNotARepo
		}
	default:
		dir, err = (&git.RealExecutor{}).Run(cwd, "rev-parse", "--show-toplevel")
		if err != nil {
			return "", ErrNotARepo
		}
	}

	// Auto-register — fire-and-forget, never block commands. Secondary checkouts
	// are filtered out later by config.IsRepo, so only primary ones persist.
	_ = config.Add(dir)

	return dir, nil
}

// managerForRepo returns the backend driving an already-known repo path. It is
// used by global commands, which walk registered repos rather than the cwd and
// therefore must never prompt.
//
// A colocated repo with no saved preference resolves to both backends so a global
// listing shows every checkout that exists; callers that must act on exactly one
// entry disambiguate from the row itself, which carries its kind.
func managersForRepo(repoPath string) []vcs.Manager {
	det, err := vcs.Detect(repoPath)
	if err != nil {
		return nil
	}

	if det.Colocated() {
		if saved, ok := config.VCSPref(repoPath); ok {
			if vcs.Available(saved) {
				return []vcs.Manager{newManager(saved)}
			}
			return nil
		}
		var mgrs []vcs.Manager
		for _, k := range det.Kinds {
			if vcs.Available(k) {
				mgrs = append(mgrs, newManager(k))
			}
		}
		return mgrs
	}

	if !vcs.Available(det.Kinds[0]) {
		return nil
	}
	return []vcs.Manager{newManager(det.Kinds[0])}
}

// validateRef checks a proposed checkout name against the backend's rules. git
// requires a valid ref name; jj only requires a usable workspace name, which is
// laxer — slashes are fine in both.
func validateRef(mgr vcs.Manager, ref string) error {
	if mgr.Kind() == vcs.KindJJ {
		return jj.ValidateRef(ref)
	}
	return git.NewBranchManager(&git.RealExecutor{}).ValidateBranchName(ref)
}

// hintOtherBackendMatch reports a query that misses under the active backend but
// would hit under the other one. In a colocated repo the target may simply live on
// the side wtf was not told to use, and saying so beats a bare "not found".
func hintOtherBackendMatch(cmd *cobra.Command, mgr vcs.Manager, dir, query string) {
	other := vcs.KindGit
	if mgr.Kind() == vcs.KindGit {
		other = vcs.KindJJ
	}

	det, err := vcs.Detect(dir)
	if err != nil || !det.Has(other) || !vcs.Available(other) {
		return
	}

	wt, err := newManager(other).Find(dir, query)
	if err != nil {
		return
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s %s exists as a %s %s here — %s\n",
		dim("note:"), cyan(wt.Branch), other.Label(), other.Noun(),
		cyan(fmt.Sprintf("wtf sw --vcs %s %s", other.Label(), query)))
}

// inferKindFromExisting picks a backend for a colocated repo by looking at which
// kind of secondary checkout it already contains.
//
// This matters because colocation is jj's *default* — `jj git init` creates both
// .git and .jj — so "both markers present" is the normal shape of a jj repo rather
// than an unusual one. Guessing git for every such repo would quietly hand jj users
// git worktrees that jj does not track.
//
// Evidence beats a coin flip, and it is backward compatible by construction: a repo
// that already has git worktrees resolves to git, exactly as it did before jj
// support. Only a repo with no secondary checkouts at all is genuinely undecidable,
// and there the choice has nothing to conflict with yet.
func inferKindFromExisting(root string) (vcs.Kind, bool) {
	countExtra := func(kind vcs.Kind) int {
		if !vcs.Available(kind) {
			return 0
		}
		wts, err := newManager(kind).List(root)
		if err != nil {
			return 0
		}
		n := 0
		for _, wt := range wts {
			// The primary checkout is shared by both backends, so only additional
			// ones are evidence of which tool is in use.
			if !wt.IsMain && !wt.IsBare {
				n++
			}
		}
		return n
	}

	gitCount := countExtra(vcs.KindGit)
	jjCount := countExtra(vcs.KindJJ)

	switch {
	case gitCount > 0 && jjCount == 0:
		return vcs.KindGit, true
	case jjCount > 0 && gitCount == 0:
		return vcs.KindJJ, true
	case gitCount > 0 && jjCount > 0:
		// Evidence for both. Refuse to guess.
		return "", false
	}

	// No secondary checkouts either way — a freshly cloned repo. Whether jj owns
	// the working copy still shows in git's HEAD.
	if jjOwnsWorkingCopy(root) {
		return vcs.KindJJ, true
	}
	return "", false
}

// jjOwnsWorkingCopy reports whether jj, rather than git, is driving the working
// copy of a colocated repo.
//
// jj leaves git's HEAD detached whenever it updates the working copy, so a
// colocated repo with a detached HEAD is one jj is managing — `jj git clone` and
// `jj commit` both produce this, while `git clone` and ordinary git use leave HEAD
// on a branch. This is what keeps a fresh `jj git clone` from being handed git
// worktrees in a non-interactive shell, where there is nobody to ask.
func jjOwnsWorkingCopy(root string) bool {
	exec := &git.RealExecutor{}

	// HEAD has to resolve before its shape means anything. Without this, a
	// directory that is not a git repo at all — or a repo with no commits yet —
	// makes symbolic-ref fail and would be misread as "detached".
	if _, err := exec.Run(root, "rev-parse", "--verify", "HEAD"); err != nil {
		return false
	}

	// A non-zero exit now means HEAD is not a symbolic ref, i.e. detached.
	_, err := exec.Run(root, "symbolic-ref", "--quiet", "HEAD")
	return err != nil
}

// pickerKindLabel returns the backend to tag local picker rows with, or an empty
// kind when tagging would be noise.
//
// A single-backend repo needs no label: every row necessarily comes from the same
// place, and stamping "(git)" on every line of a plain git repo is pure clutter.
// Only a colocated repo, where both kinds of checkout can coexist, benefits. Global
// listings always tag, because they span repos.
func pickerKindLabel(mgr vcs.Manager, dir string) vcs.Kind {
	det, err := vcs.Detect(dir)
	if err != nil || !det.Colocated() {
		return ""
	}
	return mgr.Kind()
}
