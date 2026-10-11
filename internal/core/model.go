package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Fault carries a stable application error code and message.
type Fault struct {
	Code    string
	Message string
}

func (e *Fault) Error() string { return e.Message }

// Fail constructs a coded application error.
func Fail(code, message string) error { return &Fault{code, message} }

// ErrorCode returns the application code or operation_failed for other errors.
func ErrorCode(err error) string {
	var f *Fault
	if errors.As(err, &f) {
		return f.Code
	}
	return "operation_failed"
}

// ID generates a random workspace or repository identifier.
func ID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,79}$`)

// CheckName validates a portable workspace, stage or task name.
func CheckName(name string) error {
	if !validName.MatchString(name) {
		return Fail("invalid_name", "Use 1–80 letters, digits, dots, underscores or dashes")
	}
	return nil
}

// Stage records a project command result and its input fingerprint.
type Stage struct {
	Status      string     `json:"status"`
	Fingerprint string     `json:"fingerprint"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	Error       string     `json:"error,omitempty"`
	Reused      bool       `json:"reused"`
}

// Workspace is the durable identity and lifecycle record for a JJ workspace.
type Workspace struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Path      string           `json:"path"`
	Owned     bool             `json:"owned"`
	Status    string           `json:"status"`
	Stages    map[string]Stage `json:"stages"`
	LastError string           `json:"last_error,omitempty"`
	// CreatedAt is the first WTF registration time, absent for legacy records.
	CreatedAt *time.Time `json:"created_at,omitempty"`
	Message   string     `json:"message,omitempty"`
}

// Claim records a resource identity owned by a workspace.
type Claim struct {
	Owner string `json:"owner"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

// State is the repository registry of workspaces and resource claims.
type State struct {
	Version      int                  `json:"version"`
	RepositoryID string               `json:"repository_id"`
	Anchor       string               `json:"anchor"`
	Workspaces   map[string]Workspace `json:"workspaces"`
	Claims       map[string]Claim     `json:"claims"`
}

// Repository locates an opted-in repository and its registry.
type Repository struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Store string `json:"store"`
}

// Index is the global index of opted-in repositories.
type Index struct {
	Version      int                   `json:"version"`
	Repositories map[string]Repository `json:"repositories"`
}

// App binds the current directory, registry home and subprocess output.
type App struct {
	Cwd     string
	Home    string
	Out     io.Writer
	Reviews ReviewClient
	PRs     PRClient
}

// New constructs a workspace application.
func New(cwd, home string, out io.Writer) *App {
	if out == nil {
		out = io.Discard
	}
	return &App{Cwd: cwd, Home: home, Out: out}
}

// Home resolves the configured or default WTF registry home.
func Home() (string, error) {
	if h := os.Getenv("WTF_HOME"); h != "" {
		return filepath.Abs(h)
	}
	h, e := os.UserHomeDir()
	if e != nil {
		return "", e
	}
	return filepath.Join(h, ".wtf-v1"), nil
}

// Exec runs a bounded child command with explicit environment overrides.
func (a *App) Exec(ctx context.Context, dir string, argv []string, env map[string]string, timeout time.Duration, quiet bool) error {
	if len(argv) == 0 {
		return Fail("invalid_command", "Command argv must not be empty")
	}
	c, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(c, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for k, v := range env {
		if strings.ContainsAny(k, "=\x00") || k == "" || strings.ContainsRune(v, 0) {
			return Fail("invalid_environment", "Invalid process environment input")
		}
		prefix := k + "="
		filtered := cmd.Env[:0]
		for _, s := range cmd.Env {
			if !strings.HasPrefix(s, prefix) {
				filtered = append(filtered, s)
			}
		}
		filtered = append(filtered, prefix+v)
		cmd.Env = filtered
	}
	configureCommand(cmd)
	if !quiet {
		cmd.Stdout = a.Out
		cmd.Stderr = a.Out
	}
	cmd.Stdin = nil
	err := cmd.Run()
	finishCommand(cmd)
	if c.Err() != nil {
		return Fail("command_cancelled", c.Err().Error())
	}
	if err != nil {
		return Fail("command_failed", fmt.Sprintf("%s: %v", argv[0], err))
	}
	return nil
}
func output(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "jj", args...)
	cmd.Dir = dir
	b, e := cmd.CombinedOutput()
	if e != nil {
		return "", Fail("jj_failed", strings.TrimSpace(string(b)))
	}
	return strings.TrimSpace(string(b)), nil
}
func samePath(a, b string) bool {
	aa, e := filepath.EvalSymlinks(a)
	if e != nil {
		return false
	}
	bb, e := filepath.EvalSymlinks(b)
	return e == nil && aa == bb
}
func writeAtomic(path string, v any) error {
	data, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	dir := filepath.Dir(path)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".write-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(data)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	if e = os.Rename(tmp, path); e != nil {
		return e
	}
	d, e := os.Open(dir)
	if e != nil {
		return e
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}
func readJSON(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, v); e != nil {
		return Fail("invalid_registry", fmt.Sprintf("Cannot decode %s: %v", path, e))
	}
	return nil
}
