package forge

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ReviewStatus represents the review state of a PR.
type ReviewStatus string

// Review status values for pull requests.
const (
	ReviewNone     ReviewStatus = ""
	ReviewApproved ReviewStatus = "approved"
	ReviewChanges  ReviewStatus = "changes_requested"
	ReviewPending  ReviewStatus = "pending"
)

// PR represents a pull request or merge request.
type PR struct {
	Number       int          `json:"number"`
	Title        string       `json:"title"`
	Branch       string       `json:"branch"`
	Author       string       `json:"author"`
	CreatedAt    time.Time    `json:"created_at"`
	ReviewStatus ReviewStatus `json:"review_status"`
	URL          string       `json:"url"`
	IsDraft      bool         `json:"is_draft"`
}

// Forge abstracts GitHub/GitLab API interactions.
type Forge interface {
	// ListPRs returns all open pull requests.
	ListPRs(ctx context.Context) ([]PR, error)

	// GetPR returns a specific PR by number.
	GetPR(ctx context.Context, number int) (*PR, error)

	// PRURL returns the web URL for a given PR number.
	PRURL(number int) string

	// FetchRef returns the git refspec to fetch a PR head.
	// For GitHub: "pull/{n}/head:pr-{n}"
	// For GitLab: "merge-requests/{n}/head:mr-{n}"
	FetchRef(number int) string

	// Name returns the forge name (e.g. "github", "gitlab").
	Name() string
}

// TokenFunc retrieves an auth token. Abstracted for testability.
type TokenFunc func() (string, error)

// Detect identifies the forge type from a git remote URL and returns
// an appropriate Forge implementation. Returns an error if the forge
// cannot be determined or authentication fails.
func Detect(remoteURL string, opts ...Option) (Forge, error) {
	host, owner, repo := parseRemote(remoteURL)
	if host == "" {
		return nil, fmt.Errorf("unrecognized remote URL format: %q", remoteURL)
	}

	cfg := &options{}
	for _, o := range opts {
		o(cfg)
	}

	switch {
	case strings.Contains(host, "github"):
		tokenFn := cfg.tokenFunc
		if tokenFn == nil {
			tokenFn = ghToken
		}
		return newGitHub(host, owner, repo, tokenFn)

	case strings.Contains(host, "gitlab"):
		tokenFn := cfg.tokenFunc
		if tokenFn == nil {
			tokenFn = glabToken
		}
		return newGitLab(host, owner, repo, tokenFn)

	default:
		return nil, fmt.Errorf("unsupported forge host: %q", host)
	}
}

// Option configures Detect behavior.
type Option func(*options)

type options struct {
	tokenFunc TokenFunc
}

// WithTokenFunc overrides the default token retrieval for testing.
func WithTokenFunc(fn TokenFunc) Option {
	return func(o *options) {
		o.tokenFunc = fn
	}
}

// parseRemote extracts host, owner, and repo from a git remote URL.
// Supports SSH (git@host:owner/repo.git), ssh:// and https:// formats.
func parseRemote(remote string) (host, owner, repo string) {
	remote = strings.TrimSpace(remote)

	var path string

	switch {
	case strings.HasPrefix(remote, "git@"):
		// git@host:owner/repo.git
		remote = strings.TrimPrefix(remote, "git@")
		parts := strings.SplitN(remote, ":", 2)
		if len(parts) != 2 {
			return "", "", ""
		}
		host = parts[0]
		path = parts[1]

	case strings.HasPrefix(remote, "ssh://"):
		// ssh://git@host/owner/repo
		remote = strings.TrimPrefix(remote, "ssh://")
		if idx := strings.Index(remote, "@"); idx != -1 {
			remote = remote[idx+1:]
		}
		host, path, _ = strings.Cut(remote, "/")

	case strings.HasPrefix(remote, "https://"), strings.HasPrefix(remote, "http://"):
		// https://host/owner/repo.git
		remote = strings.TrimPrefix(remote, "https://")
		remote = strings.TrimPrefix(remote, "http://")
		host, path, _ = strings.Cut(remote, "/")

	default:
		return "", "", ""
	}

	path = strings.TrimSuffix(path, ".git")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", ""
	}

	return host, parts[0], parts[1]
}
