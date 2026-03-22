package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RCFileManager handles shell RC file operations.
type RCFileManager struct {
	HomeDir string
}

// NewRCFileManager returns an RCFileManager using the real home directory.
func NewRCFileManager() (*RCFileManager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}
	return &RCFileManager{HomeDir: home}, nil
}

// RCFilePath returns the RC file path for the given shell.
func (m *RCFileManager) RCFilePath(shell Shell) (string, error) {
	switch shell {
	case Bash:
		return filepath.Join(m.HomeDir, ".bashrc"), nil
	case Zsh:
		return filepath.Join(m.HomeDir, ".zshrc"), nil
	case Fish:
		return filepath.Join(m.HomeDir, ".config", "fish", "config.fish"), nil
	default:
		return "", fmt.Errorf("unsupported shell: %q", shell)
	}
}

// InitLine returns the eval line for the given shell.
// Uses explicit shell argument like Starship: eval "$(wtf init bash)"
func InitLine(shell Shell) string {
	switch shell {
	case Fish:
		return "wtf init fish | source"
	case Zsh:
		return `eval "$(wtf init zsh)"`
	default:
		return `eval "$(wtf init bash)"`
	}
}

// IsInitPresent checks whether the wtf init line is already in the file.
func IsInitPresent(rcPath string) (bool, error) {
	data, err := os.ReadFile(rcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading rc file: %w", err)
	}
	return strings.Contains(string(data), "wtf init"), nil
}

// AppendInit appends the wtf init line to the RC file.
func AppendInit(rcPath string, shell Shell) error {
	line := InitLine(shell)
	content := fmt.Sprintf("\n# WorkTreeForge shell integration\n%s\n", line)

	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening rc file: %w", err)
	}

	if _, err := f.WriteString(content); err != nil {
		_ = f.Close()
		return fmt.Errorf("writing to rc file: %w", err)
	}

	return f.Close()
}
