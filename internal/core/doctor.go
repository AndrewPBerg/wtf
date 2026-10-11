package core

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ToolCheck describes readiness for a feature, independently of workspace health.
type ToolCheck struct {
	Tool     string            `json:"tool"`
	Code     string            `json:"code"`
	Status   string            `json:"status"`
	Required bool              `json:"required"`
	Feature  string            `json:"feature"`
	Message  string            `json:"message"`
	Remedy   string            `json:"remedy,omitempty"`
	Review   *ReviewResolution `json:"review,omitempty"`
}

type doctorProbe func(context.Context, string, string, ...string) (string, error)

// Keep subprocess output private: authentication commands can include account
// details. Diagnostics use fixed messages and never return child output.
func probeDoctor(ctx context.Context, dir, binary string, args ...string) (string, error) {
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, binary, args...)
	cmd.Dir = dir
	cmd.Stdin = nil
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	configureCommand(cmd)
	err := cmd.Run()
	finishCommand(cmd)
	if bounded.Err() != nil {
		return "", bounded.Err()
	}
	return strings.TrimSpace(out.String()), err
}

func doctorBinary(ctx context.Context, dir, name string) (string, error) {
	// Match the GitHub client: bypass Mise launchers that may run setup.
	if name == "gh" || name == "node" {
		if mise, err := exec.LookPath("mise"); err == nil {
			if candidate, err := probeDoctor(ctx, dir, mise, "which", name); err == nil && filepath.IsAbs(candidate) {
				if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
					return candidate, nil
				}
			}
		}
	}
	return exec.LookPath(name)
}

func doctorTools(ctx context.Context, dir string, lookup func(context.Context, string, string) (string, error), probe doctorProbe) []ToolCheck {
	checks := []ToolCheck{}
	jj := ToolCheck{Tool: "jj", Code: "jj_ready", Status: "ready", Required: true, Feature: "workspace lifecycle", Message: "JJ is installed and executable"}
	binary, err := lookup(ctx, dir, "jj")
	if err != nil {
		jj.Code, jj.Status, jj.Message, jj.Remedy = "jj_missing", "missing", "JJ is not available on PATH", "Install Jujutsu (jj) and make it available on PATH: https://jj-vcs.github.io/jj/latest/install-and-setup/"
	} else if _, err := probe(ctx, dir, binary, "--version"); err != nil {
		jj.Code, jj.Status, jj.Message, jj.Remedy = "jj_unavailable", "unavailable", "JJ could not be executed", "Check that jj --version succeeds"
	} else {
		for _, key := range []string{"user.name", "user.email"} {
			value, err := probe(ctx, dir, binary, "config", "get", key, "--ignore-working-copy")
			check := ToolCheck{Tool: "jj", Code: "jj_" + strings.ReplaceAll(key, ".", "_") + "_missing", Status: "unavailable", Required: true, Feature: "workspace lifecycle", Message: "JJ " + key + " is empty or could not be read", Remedy: "Set " + key + " with jj config set --user " + key + " <value>"}
			if err != nil || value == "" {
				checks = append(checks, check)
			}
		}
	}
	checks = append([]ToolCheck{jj}, checks...)
	gh := ToolCheck{Tool: "gh", Code: "gh_ready", Status: "ready", Feature: "GitHub PR and check metadata", Message: "GitHub CLI authentication is available for github.com"}
	binary, err = lookup(ctx, dir, "gh")
	if err != nil {
		gh.Code, gh.Status, gh.Message, gh.Remedy = "gh_missing", "missing", "GitHub CLI is not installed or available", "Install GitHub CLI and run gh auth login --hostname github.com: https://cli.github.com/"
	} else if _, err := probe(ctx, dir, binary, "auth", "status", "--active", "--hostname", "github.com"); err != nil {
		gh.Code, gh.Status, gh.Message, gh.Remedy = "gh_auth_unavailable", "unavailable", "GitHub authentication could not be verified; credentials, network access, or the CLI may be unavailable", "Run gh auth status --active --hostname github.com; authenticate with gh auth login --hostname github.com if needed"
	}
	return append(checks, gh)
}
