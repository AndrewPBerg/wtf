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

type Fault struct {
	Code    string
	Message string
}

func (e *Fault) Error() string        { return e.Message }
func Fail(code, message string) error { return &Fault{code, message} }
func ErrorCode(err error) string {
	var f *Fault
	if errors.As(err, &f) {
		return f.Code
	}
	return "operation_failed"
}
func ID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,79}$`)

func CheckName(name string) error {
	if !validName.MatchString(name) {
		return Fail("invalid_name", "Use 1–80 letters, digits, dots, underscores or dashes")
	}
	return nil
}

type Stage struct {
	Status      string     `json:"status"`
	Fingerprint string     `json:"fingerprint"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	Error       string     `json:"error,omitempty"`
	Reused      bool       `json:"reused"`
}
type Workspace struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Path      string           `json:"path"`
	Owned     bool             `json:"owned"`
	Status    string           `json:"status"`
	Stages    map[string]Stage `json:"stages"`
	LastError string           `json:"last_error,omitempty"`
}
type Claim struct {
	Owner string `json:"owner"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Value string `json:"value"`
}
type State struct {
	Version      int                  `json:"version"`
	RepositoryID string               `json:"repository_id"`
	Anchor       string               `json:"anchor"`
	Workspaces   map[string]Workspace `json:"workspaces"`
	Claims       map[string]Claim     `json:"claims"`
}
type Repository struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Store string `json:"store"`
}
type Index struct {
	Version      int                   `json:"version"`
	Repositories map[string]Repository `json:"repositories"`
}
type App struct {
	Cwd     string
	Home    string
	Out     io.Writer
	Reviews ReviewClient
}

func New(cwd, home string, out io.Writer) *App {
	if out == nil {
		out = io.Discard
	}
	return &App{Cwd: cwd, Home: home, Out: out}
}
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
		cmd.Env = append(filtered, prefix+v)
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
	defer os.Remove(tmp)
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
	defer d.Close()
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
