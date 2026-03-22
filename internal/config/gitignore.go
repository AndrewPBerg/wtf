package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnsureGitignore checks whether .wt-forge.toml is listed in the repo's
// .gitignore. If it is not present, it appends the entry. Returns true if
// the file was modified.
func EnsureGitignore(repoDir string) (bool, error) {
	gitignorePath := filepath.Join(repoDir, ".gitignore")

	present, err := isInGitignore(gitignorePath)
	if err != nil {
		return false, fmt.Errorf("checking .gitignore: %w", err)
	}
	if present {
		return false, nil
	}

	if err := appendToGitignore(gitignorePath, ProjectConfigFile); err != nil {
		return false, fmt.Errorf("updating .gitignore: %w", err)
	}
	return true, nil
}

// isInGitignore checks if .wt-forge.toml (or a pattern matching it) is
// already present in the .gitignore file. Returns false if the file does
// not exist.
func isInGitignore(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Exact match
		if line == ProjectConfigFile {
			return true, nil
		}
		// Common glob patterns that would cover .wt-forge.toml
		if line == ".wt-forge*" || line == ".wt-forge.*" || line == "*.toml" {
			return true, nil
		}
	}
	return false, scanner.Err()
}

// appendToGitignore adds an entry to the .gitignore file, creating it if
// necessary. It ensures a leading newline so the entry doesn't merge with
// an existing last line.
func appendToGitignore(path, entry string) error {
	var needsNewline bool

	// Check if the file exists and whether it ends with a newline.
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		needsNewline = true
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	var prefix string
	if needsNewline {
		prefix = "\n"
	}

	// Add a blank line separator + comment for clarity.
	_, err = fmt.Fprintf(f, "%s\n# wt-forge config (user-specific)\n%s\n", prefix, entry)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		return closeErr
	}
	return err
}
