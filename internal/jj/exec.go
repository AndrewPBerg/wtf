// Package jj implements the vcs.Manager interface on top of the jj CLI, mapping
// jj workspaces onto wtf's worktree model.
package jj

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/vcs"
)

// Executor abstracts jj CLI calls for testability.
type Executor interface {
	// Run executes a jj command in the given directory and returns stdout.
	Run(dir string, args ...string) (string, error)
}

// RealExecutor shells out to the jj CLI.
type RealExecutor struct{}

// Run executes a jj command in the given directory and returns stdout.
//
// --color=never is always passed so parsing never has to strip ANSI codes. jj
// writes hints and warnings (unconfigured user.name, git import notices) to
// stderr, which is deliberately not merged into the returned output.
func (r *RealExecutor) Run(dir string, args ...string) (string, error) {
	full := append([]string{"--color=never"}, args...)
	cmd := exec.Command("jj", full...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("jj %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

// GitExecutor runs git against a specific git directory. jj keeps its history in a
// git repo, but in a non-colocated repo that repo lives inside .jj where a plain
// `git` invocation cannot find it — so the directory has to be passed explicitly.
type GitExecutor interface {
	// Run executes git with --git-dir set to gitDir.
	Run(gitDir string, args ...string) (string, error)
}

// RealGitExecutor shells out to the git CLI against an explicit git directory.
type RealGitExecutor struct{}

// Run executes git with --git-dir set to gitDir.
func (r *RealGitExecutor) Run(gitDir string, args ...string) (string, error) {
	full := append([]string{"--git-dir", gitDir}, args...)
	cmd := exec.Command("git", full...)
	// --git-dir names the repo explicitly, so an inherited GIT_DIR must not win.
	cmd.Env = vcs.SanitizedEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}
