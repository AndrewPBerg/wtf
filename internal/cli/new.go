package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AndrewPBerg/wtf/internal/forge"
	"github.com/AndrewPBerg/wtf/internal/identity"
	"github.com/AndrewPBerg/wtf/internal/port"
	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/AndrewPBerg/wtf/internal/vcs"
	"github.com/spf13/cobra"
)

var (
	newBase       string
	newBranchFlag string
	newPRFlag     string
	newNoSetup    bool
	newNoEnv      bool
	newCopyEnv    bool
	newNoInstall  bool
	newNoServe    bool
	newNoGitDiff  bool
	newEnsure     bool
)

func init() {
	newCmd.Flags().StringVar(&newBase, "base", "main", "Base branch to create from")
	newCmd.Flags().StringVarP(&newBranchFlag, "branch", "b", "", "Fetch and track an existing remote branch")
	newCmd.Flags().StringVarP(&newPRFlag, "pr", "P", "", "Checkout a pull request (number, branch, or title)")
	newCmd.Flags().BoolVar(&newNoSetup, "no-setup", false, "Skip all post-create setup (env files and install)")
	newCmd.Flags().BoolVar(&newNoEnv, "no-env", false, "Skip env file handling")
	newCmd.Flags().BoolVar(&newCopyEnv, "copy-env", false, "Copy env files instead of symlinking (safer for agent worktrees)")
	newCmd.Flags().BoolVar(&newNoInstall, "no-install", false, "Skip package manager install")
	newCmd.Flags().BoolVar(&newNoServe, "no-serve", false, "Skip starting dev server")
	newCmd.Flags().BoolVar(&newNoGitDiff, "no-git-diff", false, "Skip Git metadata for editor diff views in jj workspaces")
	newCmd.Flags().BoolVar(&newEnsure, "ensure", false, "Ensure the requested workspace exists without repeating creation or setup")
	newCmd.MarkFlagsMutuallyExclusive("branch", "pr")
	newCmd.MarkFlagsMutuallyExclusive("no-env", "copy-env")

	_ = newCmd.RegisterFlagCompletionFunc("branch", completeRemoteBranchValues)
	_ = newCmd.RegisterFlagCompletionFunc("pr", completePRValues)

	rootCmd.AddCommand(newCmd)
}

var newCmd = &cobra.Command{
	Use:   "new [branch]",
	Short: "Create a new worktree for a branch",
	Long: "Create a new worktree from a branch name, remote branch, or pull request.\n\n" +
		bold("Modes") + dim(" (mutually exclusive):") + "\n" +
		"  " + cyan("wtf new <branch>") + "           Create a new branch from --base\n" +
		"  " + cyan("wtf new <number>") + "           Checkout a pull request by number (auto-detected)\n" +
		"  " + cyan("wtf new --branch <name>") + "    Fetch and track an existing remote branch\n" +
		"  " + cyan("wtf new --pr <id>") + "          Checkout a pull request by number, branch, or title\n\n" +
		bold("Setup:") + "\n" +
		"  By default, env files are symlinked from the main worktree and the\n" +
		"  detected package manager runs install. Use --copy-env for isolated\n" +
		"  agent worktrees, or --no-setup, --no-env, or --no-install to skip.",
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeRemoteBranches,
	RunE: func(cmd *cobra.Command, args []string) error {
		return dispatchNew(cmd, args, newBase, newBranchFlag, newPRFlag, false)
	},
}

// setupOptsFromFlags builds Options from the CLI flags.
func setupOptsFromFlags() setup.Options {
	opts := setup.Options{}
	if newNoSetup {
		opts.SkipEnv = true
		opts.SkipInstall = true
	}
	if newNoEnv {
		opts.SkipEnv = true
	}
	if newCopyEnv {
		opts.EnvStrategy = "copy"
	}
	if newNoInstall {
		opts.SkipInstall = true
	}
	return opts
}

// dispatchNew validates the mode and dispatches to the appropriate handler.
// switchMode controls output: when true, path goes to stdout (for shell cd),
// messages to stderr. When false, everything goes to stdout.
func dispatchNew(cmd *cobra.Command, args []string, base, branchFlag, prFlag string, switchMode bool) error {
	wm, err := resolveManager(cmd)
	if err != nil {
		return err
	}
	runner := setup.NewRunner()

	modes := 0
	if len(args) > 0 {
		modes++
	}
	if branchFlag != "" {
		modes++
	}
	if prFlag != "" {
		modes++
	}

	// An unset --base means "the main line". git names it literally; jj resolves
	// it from trunk(), so hand the jj backend an empty base and let it decide
	// rather than assuming a bookmark called "main" exists.
	if !cmd.Flags().Changed("base") && wm.Kind() == vcs.KindJJ {
		base = ""
	}
	switch {
	case modes == 0:
		return fmt.Errorf("requires a branch name, --branch, or --pr flag")
	case modes > 1:
		return fmt.Errorf("positional branch argument, --branch, and --pr are mutually exclusive")
	case branchFlag != "":
		if cmd.Flags().Changed("base") {
			return fmt.Errorf("--base cannot be used with --branch")
		}
		return runNewBranch(cmd, branchFlag, wm, runner, switchMode)
	case prFlag != "":
		if cmd.Flags().Changed("base") {
			return fmt.Errorf("--base cannot be used with --pr")
		}
		return runNewPR(cmd, prFlag, wm, runner, nil, switchMode)
	default:
		arg := args[0]
		// Auto-detect PR numbers: bare "42" or "#42" routes to PR checkout.
		trimmed := strings.TrimPrefix(arg, "#")
		if n, err := strconv.Atoi(trimmed); err == nil && n > 0 {
			if cmd.Flags().Changed("base") {
				return fmt.Errorf("--base cannot be used with a PR number")
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s Detected PR number, checking out PR #%d…\n", dim("→"), n)
			return runNewPR(cmd, arg, wm, runner, nil, switchMode)
		}
		return runNew(cmd, args[0], base, wm, runner, switchMode)
	}
}

func createdJSON(created createdWorkspace, branch string) map[string]any {
	return map[string]any{"version": 1, "path": created.Path, "branch": branch, "repository_id": created.Workspace.RepositoryID, "workspace_id": created.Workspace.ID, "name": created.Workspace.Name, "native_name": created.Workspace.NativeName}
}

func runNew(cmd *cobra.Command, branch, base string, wm vcs.Manager, runner *setup.Runner, switchMode bool) error {
	if err := validateRef(wm, branch); err != nil {
		return err
	}

	dir, err := repoDirFor(wm)
	if err != nil {
		return err
	}

	created, err := createIdentityWorkspaceWithEnsure(cmd, wm, runner, dir, branch, base, newEnsure)
	wtPath := created.Path
	if err != nil {
		return err
	}

	if jsonOutput {
		return writeJSON(cmd.OutOrStdout(), createdJSON(created, branch))
	}

	msgW, pathW := newOutputWriters(cmd, switchMode)
	if pathW != nil {
		_, _ = fmt.Fprintln(pathW, wtPath)
	}
	_, _ = fmt.Fprintf(msgW, "%s Created %s at %s\n",
		greenBold("✔"), wm.Kind().Noun(), cyan(wtPath))

	return nil
}

func runNewBranch(cmd *cobra.Command, branch string, wm vcs.Manager, runner *setup.Runner, switchMode bool) error {
	if err := validateRef(wm, branch); err != nil {
		return err
	}

	dir, err := repoDirFor(wm)
	if err != nil {
		return err
	}
	// Fetch the remote branch, creating a local tracking ref. This goes through the
	// backend because a jj repo may keep its git repo inside .jj, out of reach of a
	// plain `git fetch`.
	fetchRef := fmt.Sprintf("%s:%s", branch, branch)
	if err := wm.FetchRefspec(dir, "origin", fetchRef); err != nil {
		return fmt.Errorf("fetching remote branch %q: %w", branch, err)
	}

	created, err := createIdentityWorkspaceWithEnsure(cmd, wm, runner, dir, branch, branch, newEnsure)
	wtPath := created.Path
	if err != nil {
		return fmt.Errorf("creating worktree: %w", err)
	}

	if jsonOutput {
		return writeJSON(cmd.OutOrStdout(), createdJSON(created, branch))
	}

	msgW, pathW := newOutputWriters(cmd, switchMode)
	if pathW != nil {
		_, _ = fmt.Fprintln(pathW, wtPath)
	}
	_, _ = fmt.Fprintf(msgW, "%s Created %s at %s\n",
		greenBold("✔"), wm.Kind().Noun(), cyan(wtPath))

	return nil
}

// forgeFactory creates a Forge from a remote URL. Abstracted for testability.
type forgeFactory func(remoteURL string) (forge.Forge, error)

func runNewPR(cmd *cobra.Command, arg string, wm vcs.Manager, runner *setup.Runner, ff forgeFactory, switchMode bool) error {
	dir, err := repoDirFor(wm)
	if err != nil {
		return err
	}

	remoteURL, err := wm.RemoteURL(dir)
	if err != nil {
		return fmt.Errorf("getting remote URL: %w", err)
	}

	if ff == nil {
		ff = func(remote string) (forge.Forge, error) {
			f, fErr := forge.Detect(remote)
			if fErr != nil {
				return nil, fErr
			}
			stateDir, gcErr := wm.StateDir(dir)
			if gcErr != nil {
				return f, nil
			}
			return forge.NewCachedForge(f, stateDir), nil
		}
	}

	f, err := ff(remoteURL)
	if err != nil {
		return fmt.Errorf("detecting forge: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pr, err := resolvePR(ctx, f, arg)
	if err != nil {
		return err
	}

	stderr := cmd.ErrOrStderr()

	prLabel := fmt.Sprintf("#%d", pr.Number)
	prURL := f.PRURL(pr.Number)
	prLink := hyperlink(prURL, cyan(prLabel))

	_, _ = fmt.Fprintf(stderr, "Fetching %s %s…\n", prLink, dim(pr.Title))

	localBranch := prBranchName(f.Name(), pr.Number)
	fetchRef := f.FetchRef(pr.Number)
	if err := wm.FetchRefspec(dir, "origin", fetchRef); err != nil {
		return fmt.Errorf("fetching PR ref: %w", err)
	}

	created, err := createIdentityWorkspaceWithEnsure(cmd, wm, runner, dir, localBranch, localBranch, newEnsure)
	wtPath := created.Path
	if err != nil {
		return fmt.Errorf("creating worktree: %w", err)
	}

	if jsonOutput {
		result := createdJSON(created, localBranch)
		result["pr"] = map[string]any{"number": pr.Number, "title": pr.Title, "author": pr.Author, "url": prURL, "draft": pr.IsDraft}
		return writeJSON(cmd.OutOrStdout(), result)
	}

	msgW, pathW := newOutputWriters(cmd, switchMode)
	if pathW != nil {
		_, _ = fmt.Fprintln(pathW, wtPath)
	}
	_, _ = fmt.Fprintf(msgW, "%s Checked out %s → %s\n",
		greenBold("✔"), prLink, cyan(wtPath))

	// Background validation: verify PR is still open so we can warn about stale data.
	go func() {
		vCtx, vCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer vCancel()
		freshPR, gErr := f.GetPR(vCtx, pr.Number)
		if gErr != nil {
			return
		}
		if freshPR.IsDraft && !pr.IsDraft {
			_, _ = fmt.Fprintf(stderr, "%s PR #%d is now a draft\n", yellow("⚠"), pr.Number)
		}
	}()

	return nil
}

// repositoryResolver is intentionally narrow: repository marker/adoption policy
// belongs to the identity layer, not command orchestration.
type repositoryResolver interface {
	ResolveRepository(locator, stateDir string) (identity.Repository, error)
}

type identityLifecycle interface {
	CreateWorkspace(repositoryID, name, backend, nativeName, path string) (identity.Workspace, error)
	ActivateWorkspace(id string) (identity.Workspace, error)
	LookupWorkspace(query string) (identity.Workspace, error)
	RemoveWorkspace(id string) (identity.Workspace, error)
	MoveWorkspace(id, path string) (identity.Workspace, error)
	MarkCleanupFailed(id string) (identity.Workspace, error)
}

type storeRepositoryResolver struct{ store *identity.Store }

// identityDependencies is replaceable by focused CLI tests; production uses the
// durable store resolver above.
var identityDependencies = defaultIdentityDependencies

func (r storeRepositoryResolver) ResolveRepository(locator, stateDir string) (identity.Repository, error) {
	return r.store.EnsureRepository(locator, stateDir)
}

type storeIdentityLifecycle struct{ store *identity.Store }

func (s storeIdentityLifecycle) CreateWorkspace(repo, name, backend, native, path string) (identity.Workspace, error) {
	return s.store.CreateWorkspace(repo, name, backend, native, path)
}
func (s storeIdentityLifecycle) ActivateWorkspace(id string) (identity.Workspace, error) {
	return s.store.ActivateWorkspace(id)
}
func (s storeIdentityLifecycle) LookupWorkspace(query string) (identity.Workspace, error) {
	return s.store.LookupWorkspace(query)
}
func (s storeIdentityLifecycle) RemoveWorkspace(id string) (identity.Workspace, error) {
	return s.store.RemoveWorkspace(id)
}
func (s storeIdentityLifecycle) MoveWorkspace(id, path string) (identity.Workspace, error) {
	return s.store.MoveWorkspace(id, path)
}
func (s storeIdentityLifecycle) MarkCleanupFailed(id string) (identity.Workspace, error) {
	return s.store.MarkCleanupFailed(id)
}

func defaultIdentityDependencies() (repositoryResolver, identityLifecycle, error) {
	store, err := identity.DefaultStore()
	if err != nil {
		return nil, nil, err
	}
	return storeRepositoryResolver{store}, storeIdentityLifecycle{store}, nil
}

func canonicalWorkspaceName(repoSlug, requested string) (string, error) {
	repoSlug = strings.ToLower(filepath.Base(repoSlug))
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", fmt.Errorf("workspace name is empty")
	}
	prefix := repoSlug + "/"
	requested = strings.TrimPrefix(requested, prefix)
	// Slashes are valid nested workspace paths (for example
	// feature/frontend/login), so an unscoped request cannot be distinguished
	// from a nested feature name by slash count alone.
	name := prefix + requested
	if err := identity.ValidateName(name); err != nil {
		return "", fmt.Errorf("invalid canonical workspace name: %w", err)
	}
	return name, nil
}

type createdWorkspace struct {
	Workspace identity.Workspace
	Path      string
	Branch    string
}

// createIdentityWorkspace is the identity-critical transaction shared by new and news.
//
//nolint:unparam // kept as the non-ensure compatibility path for existing callers.
func createIdentityWorkspace(cmd *cobra.Command, wm vcs.Manager, runner *setup.Runner, dir, branch, base string) (createdWorkspace, error) {
	return createIdentityWorkspaceWithEnsure(cmd, wm, runner, dir, branch, base, false)
}

func createIdentityWorkspaceWithEnsure(cmd *cobra.Command, wm vcs.Manager, runner *setup.Runner, dir, branch, base string, ensure bool) (createdWorkspace, error) {
	main, err := wm.MainWorktree(dir)
	if err != nil {
		return createdWorkspace{}, fmt.Errorf("finding main workspace: %w", err)
	}
	stateDir, err := wm.StateDir(dir)
	if err != nil {
		return createdWorkspace{}, fmt.Errorf("finding identity state directory: %w", err)
	}
	repoResolver, lifecycle, err := identityDependencies()
	if err != nil {
		return createdWorkspace{}, err
	}
	repo, err := repoResolver.ResolveRepository(main.Path, stateDir)
	if err != nil {
		return createdWorkspace{}, fmt.Errorf("resolving repository identity: %w", err)
	}
	name, err := canonicalWorkspaceName(filepath.Base(main.Path), branch)
	if err != nil {
		return createdWorkspace{}, err
	}
	addRef := branch
	backend := identity.Git
	nativeName := branch
	if wm.Kind() == vcs.KindJJ {
		backend = identity.JJ
		addRef, nativeName = name, name
	}
	predicted := vcs.WorktreePath(main.Path, addRef)
	if ensure {
		existing, lookupErr := lifecycle.LookupWorkspace(name)
		if lookupErr == nil {
			if existing.LifecycleState != identity.Active || existing.RepositoryID != repo.ID || existing.Backend != string(backend) || existing.NativeName != nativeName || existing.Path != predicted {
				return createdWorkspace{}, fmt.Errorf("workspace %s exists with incompatible identity or lifecycle state", name)
			}
			return createdWorkspace{Workspace: existing, Path: existing.Path, Branch: branch}, nil
		}
	}
	pending, err := lifecycle.CreateWorkspace(repo.ID, name, string(backend), nativeName, predicted)
	if err != nil {
		return createdWorkspace{}, fmt.Errorf("claiming workspace identity: %w", err)
	}
	wtPath, err := wm.Add(dir, addRef, base)
	if err != nil {
		if _, removeErr := lifecycle.RemoveWorkspace(pending.ID); removeErr != nil {
			_, _ = lifecycle.MarkCleanupFailed(pending.ID)
			return createdWorkspace{}, fmt.Errorf("creating workspace: %w (identity cleanup failed: %v)", err, removeErr)
		}
		return createdWorkspace{}, err
	}
	cleanup := func(cause error) error {
		if removeErr := wm.Remove(dir, addRef, dir, false); removeErr != nil {
			_, _ = lifecycle.MarkCleanupFailed(pending.ID)
			return fmt.Errorf("%w (workspace rollback failed: %v; identity retained as cleanup_failed)", cause, removeErr)
		}
		if _, removeErr := lifecycle.RemoveWorkspace(pending.ID); removeErr != nil {
			_, _ = lifecycle.MarkCleanupFailed(pending.ID)
			return fmt.Errorf("%w (identity cleanup failed: %v)", cause, removeErr)
		}
		return cause
	}
	if err := initWorkspaceGitDiff(wm, wtPath); err != nil {
		return createdWorkspace{}, cleanup(err)
	}
	actualPath := filepath.Clean(wtPath)
	if resolved, err := filepath.EvalSymlinks(actualPath); err == nil {
		actualPath = filepath.Clean(resolved)
	}
	if actualPath != filepath.Clean(predicted) {
		if _, err := lifecycle.MoveWorkspace(pending.ID, actualPath); err != nil {
			return createdWorkspace{}, cleanup(fmt.Errorf("recording workspace path: %w", err))
		}
	}
	active, err := lifecycle.ActivateWorkspace(pending.ID)
	if err != nil {
		return createdWorkspace{}, cleanup(fmt.Errorf("activating workspace identity: %w", err))
	}
	// Setup is outside the identity-critical transaction and runs exactly once.
	runPostCreateSetup(cmd, wm, runner, dir, wtPath)
	return createdWorkspace{Workspace: active, Path: wtPath, Branch: branch}, nil
}

func initWorkspaceGitDiff(wm vcs.Manager, wtPath string) error {
	if wm.Kind() != vcs.KindJJ || newNoGitDiff || envDisabled("WTF_JJ_GIT_DIFF") {
		return nil
	}
	manager, ok := wm.(vcs.GitDiffManager)
	if !ok {
		return fmt.Errorf("git diff metadata is not supported by the active backend")
	}
	if err := manager.InitGitDiff(wtPath); err != nil {
		return fmt.Errorf("workspace created at %s, but Git diff setup failed: %w", wtPath, err)
	}
	return nil
}

func envDisabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "0", "false", "no", "off":
		return true
	default:
		return false
	}
}

// newOutputWriters returns writers for messages and path output.
// In switch mode: messages go to stderr, path goes to stdout.
// In normal mode: messages go to stdout, path is nil (not printed).
func newOutputWriters(cmd *cobra.Command, switchMode bool) (msgW io.Writer, pathW io.Writer) {
	if switchMode {
		return cmd.ErrOrStderr(), cmd.OutOrStdout()
	}
	return cmd.OutOrStdout(), nil
}

// runPostCreateSetup runs project setup after worktree creation.
// By default, handles env files and auto-detects + runs package install.
func runPostCreateSetup(cmd *cobra.Command, wm vcs.Manager, runner *setup.Runner, dir, wtPath string) {
	if runner == nil {
		return
	}

	mainWt, mainErr := wm.MainWorktree(dir)
	if mainErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s setup skipped: %v\n", yellow("⚠"), mainErr)
		return
	}

	runner.Out = cmd.ErrOrStderr()

	opts := setupOptsFromFlags()
	if setupErr := runner.RunSetup(mainWt.Path, wtPath, opts); setupErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s setup failed: %v\n", yellow("⚠"), setupErr)
	}

	allocatePortForWorktree(cmd, wm, dir, wtPath)
}

// allocatePortForWorktree allocates a unique dev-server port for the worktree
// branch, starts the dev server (unless --no-serve), and prints status to stderr.
// Failures are non-fatal warnings.
func allocatePortForWorktree(cmd *cobra.Command, mgr vcs.Manager, repoDir, wtPath string) {
	branch, err := mgr.CurrentRef(wtPath)
	if err != nil {
		return
	}

	alloc, err := portAllocator(mgr, repoDir)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s port allocation failed: %v\n", yellow("⚠"), err)
		return
	}

	p, err := alloc.Allocate(branch)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s port allocation failed: %v\n", yellow("⚠"), err)
		return
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s PORT=%d\n", dim("port:"), p)

	if newNoServe {
		return
	}

	result, sErr := port.StartDevServer(wtPath, p)
	if sErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s dev server failed: %v\n", yellow("⚠"), sErr)
		return
	}
	if result != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s %s (pid %d, log %s)\n",
			dim("serve:"), cyan(result.Command), result.PID, dim(result.LogFile))
	}
}

// resolvePR finds a PR by number, branch name, or title.
func resolvePR(ctx context.Context, f forge.Forge, arg string) (*forge.PR, error) {
	if num, err := strconv.Atoi(strings.TrimPrefix(arg, "#")); err == nil && num > 0 {
		pr, err := f.GetPR(ctx, num)
		if err != nil {
			return nil, fmt.Errorf("PR #%d not found: %w", num, err)
		}
		return pr, nil
	}

	prs, err := f.ListPRs(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing PRs: %w", err)
	}

	for i := range prs {
		if prs[i].Branch == arg {
			return &prs[i], nil
		}
	}

	var matches []*forge.PR
	for i := range prs {
		if strings.Contains(prs[i].Branch, arg) {
			matches = append(matches, &prs[i])
		}
	}

	// Fall through to title matching if no branch matches
	if len(matches) == 0 {
		argLower := strings.ToLower(arg)
		// First try substring match on title
		for i := range prs {
			if strings.Contains(strings.ToLower(prs[i].Title), argLower) {
				matches = append(matches, &prs[i])
			}
		}
		// If still no matches, try fuzzy match on title
		if len(matches) == 0 {
			for i := range prs {
				if fuzzyScore(strings.ToLower(prs[i].Title), argLower) > 0 {
					matches = append(matches, &prs[i])
				}
			}
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no open PR found matching %q", arg)
	case 1:
		return matches[0], nil
	default:
		var names []string
		for _, m := range matches {
			names = append(names, fmt.Sprintf("#%d (%s)", m.Number, m.Branch))
		}
		return nil, fmt.Errorf("multiple PRs match %q: %s", arg, strings.Join(names, ", "))
	}
}

// prBranchName returns the local branch name for a PR.
func prBranchName(forgeName string, number int) string {
	switch forgeName {
	case "gitlab":
		return fmt.Sprintf("mr-%d", number)
	default:
		return fmt.Sprintf("pr-%d", number)
	}
}
