package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/git"
	"github.com/AndrewPBerg/wtf/internal/port"
	"github.com/AndrewPBerg/wtf/internal/vcs"
)

var (
	reCobraArgs    = regexp.MustCompile(`^accepts (?:at most )?(\d+) arg\(s\), received (\d+)$`)
	reCobraUnknown = regexp.MustCompile(`^unknown command "([^"]+)" for "([^"]+)"$`)
)

// ErrNotARepo is returned when the current directory is not inside a repository
// wtf can manage.
var ErrNotARepo = vcs.ErrNotARepo

// getRepoDir returns the root of the current checkout, whichever backend it
// belongs to. It never prompts; commands that need to disambiguate a colocated
// repo do so via resolveManager before calling into this.
func getRepoDir() (string, error) {
	mgr, err := resolveManagerQuiet()
	if err != nil {
		return "", err
	}
	return repoDirFor(mgr)
}

// suggestCommands returns command names similar to the given unknown command.
func suggestCommands(unknown string) []string {
	known := []string{"sw", "swg", "new", "news", "git-diff", "rm", "rmg", "watch", "repos", "init", "setup", "config", "completion", "register", "unregister"}
	unknown = strings.ToLower(unknown)
	var suggestions []string
	for _, cmd := range known {
		if levenshtein(unknown, cmd) <= 2 {
			suggestions = append(suggestions, cmd)
		}
	}
	return suggestions
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev = curr
	}
	return prev[lb]
}

// portAllocator creates a port.Allocator for the given repo directory. The store
// path comes from the backend so git and jj each keep it somewhere every checkout
// of the repo agrees on (.git/wtf vs .jj/repo/wtf).
func portAllocator(mgr vcs.Manager, repoDir string) (*port.Allocator, error) {
	stateDir, err := mgr.StateDir(repoDir)
	if err != nil {
		return nil, err
	}

	storePath := filepath.Join(stateDir, "ports.json")
	base := port.DetectBasePort(repoDir)
	return port.New(base, port.NewFileStore(storePath)), nil
}

// FormatError returns a user-friendly, WTF-themed error string.
func FormatError(err error) string {
	switch {
	case errors.Is(err, ErrNotARepo):
		return fmt.Sprintf(
			"%s you're not in a git or jj repo\n  %s cd into a repo, or run %s or %s to get started",
			redBold("wtf?"),
			dim("hint:"),
			cyan("git init"),
			cyan("jj git init --colocate"),
		)

	case errors.Is(err, git.ErrWorktreeNotFound):
		return fmt.Sprintf(
			"%s couldn't find that worktree\n  %s run %s to see available worktrees",
			redBold("wtf?"),
			dim("hint:"),
			cyan("wtf sw"),
		)

	case errors.Is(err, git.ErrMultipleMatches):
		return fmt.Sprintf("%s %s", redBold("wtf?"), err.Error())

	case errors.Is(err, git.ErrBranchAlreadyExists):
		return fmt.Sprintf("%s %s", redBold("wtf?"), err.Error())

	case errors.Is(err, git.ErrPathAlreadyExists):
		return fmt.Sprintf(
			"%s something's already squatting at that path\n  %s %s\n  %s nuke it or pick a different name",
			redBold("wtf?"),
			dim("path:"),
			err.Error(),
			dim("hint:"),
		)

	case errors.Is(err, git.ErrInvalidBranchName):
		return fmt.Sprintf(
			"%s %s\n  %s branch names must be valid git refs",
			redBold("wtf?"),
			err.Error(),
			dim("hint:"),
		)

	case errors.Is(err, git.ErrWorktreeHasChanges):
		return fmt.Sprintf(
			"%s worktree has uncommitted changes\n  %s run with %s to remove anyway",
			redBold("wtf?"),
			dim("hint:"),
			cyan("--force"),
		)

	case errors.Is(err, git.ErrMainWorktree):
		return fmt.Sprintf(
			"%s %s\n  %s the main checkout holds the repo itself — remove a secondary one instead",
			redBold("wtf?"),
			err.Error(),
			dim("hint:"),
		)

	default:
		msg := err.Error()

		// Cobra arg-count errors: "accepts 1 arg(s), received 0"
		if m := reCobraArgs.FindStringSubmatch(msg); m != nil {
			if m[2] == "0" {
				return fmt.Sprintf(
					"%s missing required argument\n  %s run %s to see usage",
					redBold("wtf?"),
					dim("hint:"),
					cyan("wtf --help"),
				)
			}
			return fmt.Sprintf(
				"%s too many arguments (expected %s, got %s)\n  %s run %s to see usage",
				redBold("wtf?"),
				m[1], m[2],
				dim("hint:"),
				cyan("wtf --help"),
			)
		}

		// Cobra unknown command: `unknown command "foo" for "wtf"`
		if m := reCobraUnknown.FindStringSubmatch(msg); m != nil {
			out := fmt.Sprintf(
				"%s %s is not a wtf command\n  %s run %s to see available commands",
				redBold("wtf?"),
				cyan(m[1]),
				dim("hint:"),
				cyan("wtf --help"),
			)
			// Check for similar commands via Levenshtein on known names
			if suggestions := suggestCommands(m[1]); len(suggestions) > 0 {
				out += fmt.Sprintf("\n\n%s\n", dim("Did you mean?"))
				for _, s := range suggestions {
					out += fmt.Sprintf("  %s %s\n", yellow("→"), cyan(s))
				}
			}
			return out
		}

		// Cobra unknown flag: `unknown flag: --foo` or `unknown shorthand flag: 'x'`
		if strings.HasPrefix(msg, "unknown flag:") || strings.HasPrefix(msg, "unknown shorthand flag:") {
			return fmt.Sprintf(
				"%s %s\n  %s run %s to see available flags",
				redBold("wtf?"),
				msg,
				dim("hint:"),
				cyan("wtf --help"),
			)
		}

		return fmt.Sprintf("%s %s", redBold("wtf?"), msg)
	}
}
