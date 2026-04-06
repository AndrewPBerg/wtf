package setup

import (
	"fmt"
	"os"
	"strings"
)

// Shell represents a supported shell type.
type Shell string

// Supported shell types.
const (
	Bash Shell = "bash"
	Zsh  Shell = "zsh"
	Fish Shell = "fish"
)

// ShellDetector detects the user's current shell.
type ShellDetector struct {
	// GetEnv returns the value of an environment variable.
	GetEnv func(string) string
	// ReadParentComm reads the parent process comm file (e.g. /proc/<pid>/comm).
	ReadParentComm func(pid int) (string, error)
}

// NewShellDetector returns a ShellDetector using real OS functions.
func NewShellDetector() *ShellDetector {
	return &ShellDetector{
		GetEnv:         os.Getenv,
		ReadParentComm: defaultReadParentComm,
	}
}

func defaultReadParentComm(pid int) (string, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return "", fmt.Errorf("reading parent process comm: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// Detect determines the current shell. Priority: override arg → $SHELL env → parent process.
func (d *ShellDetector) Detect(override string) (Shell, error) {
	if override != "" {
		return parseShell(override)
	}

	if envShell := d.GetEnv("SHELL"); envShell != "" {
		base := shellBasename(envShell)
		return parseShell(base)
	}

	if d.ReadParentComm != nil {
		ppid := os.Getppid()
		comm, err := d.ReadParentComm(ppid)
		if err == nil {
			base := shellBasename(comm)
			s, err := parseShell(base)
			if err == nil {
				return s, nil
			}
		}
	}

	return "", fmt.Errorf("could not detect shell: set $SHELL or pass shell name as argument")
}

// ParseShellName parses a shell name string into a Shell type.
func ParseShellName(name string) (Shell, error) {
	return parseShell(name)
}

func parseShell(name string) (Shell, error) {
	switch strings.TrimPrefix(strings.ToLower(name), "-") {
	case "bash":
		return Bash, nil
	case "zsh":
		return Zsh, nil
	case "fish":
		return Fish, nil
	default:
		return "", fmt.Errorf("unsupported shell: %q (supported: bash, zsh, fish)", name)
	}
}

func shellBasename(path string) string {
	i := strings.LastIndex(path, "/")
	if i >= 0 {
		return path[i+1:]
	}
	return path
}

// ShellFunc defines a shell function to be emitted by `wtf init`.
type ShellFunc struct {
	Name string
	Bash string // bash/zsh share syntax
	Fish string
}

// DefaultFuncs returns the default set of shell functions.
func DefaultFuncs() []ShellFunc {
	return []ShellFunc{
		{
			Name: "wtf",
			Bash: `wtf() { case "$1" in sw|swg|news) if [ ! -t 1 ]; then command wtf "$@"; return; fi; local _c="$1"; shift; local _p; _p="$(command wtf "$_c" "$@")" || return 1; [ -z "$_p" ] && return 0; builtin cd "$_p"; local _port; _port="$(command wtf port 2>/dev/null)"; [ -n "$_port" ] && export PORT="$_port"; return;; *) command wtf "$@";; esac; }`,
			Fish: `function wtf; if contains -- "$argv[1]" sw swg news; if not isatty stdout; command wtf $argv; return; end; set -l _c $argv[1]; set -l _p (command wtf $_c $argv[2..]); or return 1; if test -z "$_p"; return 0; end; builtin cd "$_p"; set -l _port (command wtf port 2>/dev/null); if test -n "$_port"; set -gx PORT $_port; end; return; else command wtf $argv; end; end`,
		},
	}
}

// CompletionRenderer generates a shell completion script for the given shell.
// Used by Render to optionally append tab-completion support.
type CompletionRenderer func(shell Shell) (string, error)

// Render emits all shell functions for the given shell.
// If cr is non-nil, it also appends shell completion scripts.
func Render(shell Shell, funcs []ShellFunc, cr CompletionRenderer) string {
	var b strings.Builder
	for i, f := range funcs {
		if i > 0 {
			b.WriteString("\n")
		}
		switch shell {
		case Fish:
			b.WriteString(f.Fish)
		default: // bash, zsh
			b.WriteString(f.Bash)
		}
		b.WriteString("\n")
	}

	if cr != nil {
		completion, err := cr(shell)
		if err == nil && completion != "" {
			b.WriteString("\n# wtf completions\n")
			b.WriteString(completion)
		}
	}

	return b.String()
}
