package setup

import (
	"fmt"
	"os"
	"path/filepath"
)

// CompletionStatus represents whether completions are configured for a shell.
type CompletionStatus struct {
	Configured bool
	Method     string // "inline", "file", ""
	Path       string // file path if method is "file"
}

// IsCompletionConfigured checks if wtf completions are set up for the given shell.
// It checks two sources: the RC file (inline via "wtf init") and standard completion file locations.
func IsCompletionConfigured(shell Shell, rcPath string, homeDir string) (CompletionStatus, error) {
	// Check 1: Is "wtf init" in the RC file? If so, completions are
	// embedded inline (since wtf init now includes them).
	present, err := IsInitPresent(rcPath)
	if err != nil {
		return CompletionStatus{}, fmt.Errorf("checking completion status: %w", err)
	}
	if present {
		return CompletionStatus{Configured: true, Method: "inline"}, nil
	}

	// Check 2: Is there a completion file in standard locations?
	path := CompletionFilePath(shell, homeDir)
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			return CompletionStatus{Configured: true, Method: "file", Path: path}, nil
		}
	}

	return CompletionStatus{}, nil
}

// CompletionFilePath returns the standard user-local completion file path for the given shell.
func CompletionFilePath(shell Shell, homeDir string) string {
	switch shell {
	case Bash:
		return filepath.Join(homeDir, ".local", "share", "bash-completion", "completions", "wtf")
	case Zsh:
		return filepath.Join(homeDir, ".zsh", "completions", "_wtf")
	case Fish:
		return filepath.Join(homeDir, ".config", "fish", "completions", "wtf.fish")
	default:
		return ""
	}
}

// WriteCompletionFile writes a completion script to the standard user-local path.
// It creates parent directories as needed.
func WriteCompletionFile(shell Shell, homeDir string, content string) (string, error) {
	path := CompletionFilePath(shell, homeDir)
	if path == "" {
		return "", fmt.Errorf("unsupported shell for completion file: %q", shell)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating completion directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("writing completion file: %w", err)
	}

	return path, nil
}
