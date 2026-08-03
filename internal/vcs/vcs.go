// Package vcs defines the version-control abstraction shared by the git and jj
// backends. wtf presents one command surface over both: a git worktree and a jj
// workspace are the same idea — a sibling checkout of the same repo — so both
// reduce to the Worktree model below.
//
// The mapping is deliberately thin. For jj, the workspace *name* plays the role
// git gives the branch name: jj enforces workspace-name uniqueness, so it is a
// valid primary key for lookup, removal, and port allocation. Bookmarks are
// carried as display-only metadata and are never created implicitly — that stays
// the user's call via `jj bookmark create` or `jj git push -c`.
package vcs

import (
	"errors"
	"fmt"
)

// Kind identifies which version control system backs a repo.
type Kind string

// Supported backends.
const (
	KindGit Kind = "git"
	KindJJ  Kind = "jj"
)

// Label returns the human-facing name of the backend.
func (k Kind) Label() string {
	switch k {
	case KindGit:
		return "git"
	case KindJJ:
		return "jj"
	default:
		return string(k)
	}
}

// Noun returns the backend's word for a secondary checkout: git calls it a
// worktree, jj calls it a workspace. Used in CLI output so the two are never
// confused for one another.
func (k Kind) Noun() string {
	switch k {
	case KindJJ:
		return "workspace"
	default:
		return "worktree"
	}
}

// RefNoun returns the backend's word for what identifies a checkout. git keys
// on the branch name; jj keys on the workspace name.
func (k Kind) RefNoun() string {
	switch k {
	case KindJJ:
		return "workspace"
	default:
		return "branch"
	}
}

// ParseKind converts a user-supplied string into a Kind.
func ParseKind(s string) (Kind, error) {
	switch s {
	case "git":
		return KindGit, nil
	case "jj":
		return KindJJ, nil
	default:
		return "", fmt.Errorf("%w: %q (want \"git\" or \"jj\")", ErrUnknownKind, s)
	}
}

// Sentinel errors shared across backends. The git and jj managers wrap these so
// internal/cli can render one set of user-facing messages for both.
var (
	ErrWorktreeNotFound     = errors.New("no matching worktree found")
	ErrMultipleMatches      = errors.New("multiple worktrees match query")
	ErrMainWorktree         = errors.New("cannot remove main worktree")
	ErrWorktreeIsCurrentDir = errors.New("cannot remove worktree for the currently checked out branch")
	ErrBranchAlreadyInUse   = errors.New("branch is already checked out")
	ErrPathAlreadyExists    = errors.New("worktree path already exists")
	ErrWorktreeHasChanges   = errors.New("worktree has uncommitted changes")
	ErrNotARepo             = errors.New("not a git or jj repository")
	ErrUnknownKind          = errors.New("unknown version control system")
	ErrInvalidRef           = errors.New("invalid name")
)

// Worktree is a single checkout of a repo — a git worktree or a jj workspace.
type Worktree struct {
	Path string `json:"path"`
	// Branch is the checkout's identifying name: a git branch name, or a jj
	// workspace name. Empty for detached or bare git worktrees.
	Branch     string `json:"branch"`
	Head       string `json:"head"`
	IsMain     bool   `json:"is_main"`
	IsBare     bool   `json:"is_bare"`
	IsDetached bool   `json:"is_detached"`
	Prunable   bool   `json:"prunable"`

	// VCS records which backend produced this entry. It matters for global
	// commands, where one listing can span git and jj repos at once.
	VCS Kind `json:"vcs,omitempty"`

	// ChangeID is jj's stable change identifier. Empty for git.
	ChangeID string `json:"change_id,omitempty"`
	// Bookmarks are the jj bookmarks pointing at this workspace's working-copy
	// commit. Display-only; wtf never creates them. Empty for git.
	Bookmarks []string `json:"bookmarks,omitempty"`
}

// Manager is the set of worktree operations wtf needs from a backend.
type Manager interface {
	// Kind reports which backend this manager drives.
	Kind() Kind

	// List returns every checkout of the repo at dir, main worktree first.
	List(dir string) ([]Worktree, error)

	// MainWorktree returns the primary checkout — the one holding the repo.
	MainWorktree(dir string) (Worktree, error)

	// Find resolves a query to exactly one checkout, matching Branch exactly
	// first and falling back to substring match.
	Find(dir, query string) (Worktree, error)

	// Add creates a checkout named ref, based on base, and returns its path.
	Add(dir, ref, base string) (string, error)

	// Remove deletes the checkout named ref. cwd is the caller's working
	// directory; removal is refused when cwd sits inside the target.
	Remove(dir, ref, cwd string, force bool) error

	// RemoteURL returns the origin remote URL, for forge/PR integration.
	RemoteURL(dir string) (string, error)

	// StateDir returns the repo-local directory wtf stores state in (allocated
	// ports, forge cache, watch state). Shared by every checkout of the repo.
	StateDir(dir string) (string, error)

	// CurrentRef returns the identifying name of the checkout at dir — the
	// current git branch, or the jj workspace name.
	CurrentRef(dir string) (string, error)

	// FetchRefspec fetches a "src:dst" refspec from a remote and makes dst usable
	// as a base for Add. Both backends store history in git, but jj repos may keep
	// that git repo inside .jj where plain git cannot reach it, so fetching has to
	// go through the backend rather than shelling out to git directly.
	FetchRefspec(dir, remote, refspec string) error

	// Cleanable returns checkouts that are safe to discard: work that has landed
	// on the main line, plus registrations whose directory is gone. Each backend
	// decides what "landed" means, since git merges branches while jj tracks
	// whether a change is already contained in trunk.
	Cleanable(dir string) ([]Worktree, error)
}
