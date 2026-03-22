package git

import (
	"fmt"
	"strings"
)

// RemoteURL returns the URL of the "origin" remote for the repo at dir.
func (wm *WorktreeManager) RemoteURL(dir string) (string, error) {
	out, err := wm.executor.Run(dir, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("getting remote URL: %w", err)
	}
	return out, nil
}

// CommitURL builds a web URL for the given commit hash from a git remote URL.
// Supports GitHub and GitLab, both SSH and HTTPS formats.
// Returns empty string if the remote URL format is not recognized.
func CommitURL(remoteURL, commitHash string) string {
	baseURL := remoteToBaseURL(remoteURL)
	if baseURL == "" {
		return ""
	}
	return baseURL + "/commit/" + commitHash
}

// remoteToBaseURL converts a git remote URL to an HTTPS base URL.
// Examples:
//
//	git@github.com:user/repo.git      → https://github.com/user/repo
//	https://github.com/user/repo.git  → https://github.com/user/repo
//	ssh://git@github.com/user/repo    → https://github.com/user/repo
func remoteToBaseURL(remote string) string {
	remote = strings.TrimSpace(remote)

	// SSH format: git@host:user/repo.git
	if strings.HasPrefix(remote, "git@") {
		remote = strings.TrimPrefix(remote, "git@")
		// Split on first ':'
		parts := strings.SplitN(remote, ":", 2)
		if len(parts) != 2 {
			return ""
		}
		host := parts[0]
		path := strings.TrimSuffix(parts[1], ".git")
		return "https://" + host + "/" + path
	}

	// ssh:// format: ssh://git@host/user/repo
	if strings.HasPrefix(remote, "ssh://") {
		remote = strings.TrimPrefix(remote, "ssh://")
		// Strip user@ prefix
		if idx := strings.Index(remote, "@"); idx != -1 {
			remote = remote[idx+1:]
		}
		remote = strings.TrimSuffix(remote, ".git")
		return "https://" + remote
	}

	// HTTPS format: https://host/user/repo.git
	if strings.HasPrefix(remote, "https://") || strings.HasPrefix(remote, "http://") {
		remote = strings.TrimSuffix(remote, ".git")
		return remote
	}

	return ""
}
