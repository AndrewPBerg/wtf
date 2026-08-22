package jj

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AndrewPBerg/wtf/internal/vcs"
)

// InitGitDiff creates a private Git metadata directory in a secondary jj
// workspace. Git-aware editors can then render the workspace change while jj
// remains the only VCS managing the shared repository.
func (m *WorkspaceManager) InitGitDiff(workspacePath string) error {
	if _, err := os.Stat(filepath.Join(workspacePath, ".git")); err == nil {
		return fmt.Errorf("%s already contains .git", workspacePath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking %s: %w", filepath.Join(workspacePath, ".git"), err)
	}

	gitDir, err := GitDir(workspacePath)
	if err != nil {
		return fmt.Errorf("finding jj's backing Git repository: %w", err)
	}
	base, err := m.gitDiffBase(workspacePath)
	if err != nil {
		return err
	}
	objectFormat, err := m.gitExec.Run(gitDir, "rev-parse", "--show-object-format")
	if err != nil {
		return fmt.Errorf("reading jj Git object format: %w", err)
	}
	objectFormat = strings.TrimSpace(objectFormat)
	if objectFormat != "sha1" && objectFormat != "sha256" {
		return fmt.Errorf("unsupported jj Git object format %q", objectFormat)
	}

	initArgs := []string{"init", "--quiet"}
	if objectFormat != "sha1" {
		initArgs = append(initArgs, "--object-format="+objectFormat)
	}
	if _, err := runWorkspaceGit(workspacePath, initArgs...); err != nil {
		return err
	}

	shadowDir := filepath.Join(workspacePath, ".git")
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(shadowDir)
		}
	}()

	if err := writeGitAlternates(workspacePath, gitDir); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(shadowDir, "info", "exclude"), []byte("/.jj/\n"), 0o644); err != nil {
		return fmt.Errorf("excluding jj metadata from Git diff: %w", err)
	}
	if err := os.WriteFile(filepath.Join(shadowDir, vcs.JJGitDiffMarker), []byte("managed by wtf; jj remains authoritative\n"), 0o644); err != nil {
		return fmt.Errorf("marking Git diff metadata: %w", err)
	}
	if _, err := runWorkspaceGit(workspacePath, "config", "--local", "gc.auto", "0"); err != nil {
		return err
	}
	if err := resetGitDiffBase(workspacePath, base); err != nil {
		return err
	}

	cleanup = false
	return nil
}

// RefreshGitDiff resets an existing shadow Git index to the current jj parent.
// It never changes working-copy files, but intentionally clears Git staging:
// staging is presentation-only here and jj remains authoritative.
func (m *WorkspaceManager) RefreshGitDiff(workspacePath string) error {
	marker := filepath.Join(workspacePath, ".git", vcs.JJGitDiffMarker)
	if _, err := os.Stat(marker); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no WTF Git diff metadata in %s", workspacePath)
		}
		return fmt.Errorf("checking Git diff metadata: %w", err)
	}
	gitDir, err := GitDir(workspacePath)
	if err != nil {
		return fmt.Errorf("finding jj's backing Git repository: %w", err)
	}
	base, err := m.gitDiffBase(workspacePath)
	if err != nil {
		return err
	}
	// jj workspaces can be moved together because their links are relative. Keep
	// the Git alternates path equally repairable when the workspace is refreshed.
	if err := writeGitAlternates(workspacePath, gitDir); err != nil {
		return err
	}
	return resetGitDiffBase(workspacePath, base)
}

func writeGitAlternates(workspacePath, gitDir string) error {
	infoDir := filepath.Join(workspacePath, ".git", "objects", "info")
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		return fmt.Errorf("creating Git alternates directory: %w", err)
	}
	objectsDir := filepath.Join(gitDir, "objects")
	if err := os.WriteFile(filepath.Join(infoDir, "alternates"), []byte(objectsDir+"\n"), 0o644); err != nil {
		return fmt.Errorf("linking jj's Git objects: %w", err)
	}
	return nil
}

type gitDiffBase struct {
	commit string
	root   bool
}

// GitDiffBaseCommit returns the single JJ parent used as the Git shadow baseline.
// An empty commit represents JJ's virtual root.
func (m *WorkspaceManager) GitDiffBaseCommit(workspacePath string) (string, error) {
	base, err := m.gitDiffBase(workspacePath)
	if err != nil {
		return "", err
	}
	return base.commit, nil
}

func (m *WorkspaceManager) gitDiffBase(workspacePath string) (gitDiffBase, error) {
	out, err := m.executor.Run(workspacePath, "log", "--ignore-working-copy", "--no-graph",
		"-r", "@-", "-T", `if(root, "root", commit_id) ++ "\n"`)
	if err != nil {
		return gitDiffBase{}, fmt.Errorf("resolving jj parent for Git diff: %w", err)
	}
	parents := strings.Fields(out)
	if len(parents) == 0 {
		return gitDiffBase{}, fmt.Errorf("jj parent for Git diff did not resolve")
	}
	if len(parents) != 1 {
		return gitDiffBase{}, fmt.Errorf("git editor diff requires exactly one jj parent; found %d", len(parents))
	}
	if parents[0] == "root" {
		return gitDiffBase{root: true}, nil
	}
	return gitDiffBase{commit: parents[0]}, nil
}

func resetGitDiffBase(workspacePath string, base gitDiffBase) error {
	if base.root {
		if _, err := runWorkspaceGit(workspacePath, "symbolic-ref", "HEAD", "refs/heads/wtf-jj-base"); err != nil {
			return err
		}
		if _, err := runWorkspaceGit(workspacePath, "read-tree", "--empty"); err != nil {
			return err
		}
		return nil
	}
	if _, err := runWorkspaceGit(workspacePath, "update-ref", "--no-deref", "HEAD", base.commit); err != nil {
		return err
	}
	if _, err := runWorkspaceGit(workspacePath, "read-tree", "--reset", base.commit); err != nil {
		return err
	}
	return nil
}

func runWorkspaceGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = vcs.SanitizedEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}
