package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// StageOptions controls command inputs, reuse checks and execution limits.
type StageOptions struct {
	Inputs         []string          `json:"inputs"`
	Check          []string          `json:"check"`
	Env            map[string]string `json:"-"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	Reuse          bool              `json:"reuse"`
	Cwd            string            `json:"cwd,omitempty"`
	EnvUnset       []string          `json:"-"`
	NoInheritEnv   bool              `json:"-"`
}

func fingerprint(ctx context.Context, root string, argv []string, o StageOptions) (string, error) {
	h := sha256.New()
	b, e := json.Marshal(struct {
		Argv         []string
		Env          map[string]string
		Inputs       []string
		Check        []string
		Timeout      int
		Cwd          string
		Unset        []string
		NoInheritEnv bool
	}{argv, o.Env, o.Inputs, o.Check, o.TimeoutSeconds, o.Cwd, o.EnvUnset, o.NoInheritEnv})
	if e != nil {
		return "", e
	}
	_, _ = h.Write(b)
	inputs := append([]string{}, o.Inputs...)
	sort.Strings(inputs)
	for _, input := range inputs {
		p, e := relative(root, input)
		if e != nil {
			return "", e
		}
		e = filepath.WalkDir(p, func(path string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if d.Type()&os.ModeSymlink != 0 {
				return Fail("invalid_path", "Stage inputs may not contain symlinks")
			}
			if d.IsDir() {
				if d.Name() == ".jj" || d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			info, e := d.Info()
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() {
				return Fail("invalid_path", "Stage inputs must be regular files")
			}
			rel, e := filepath.Rel(root, path)
			if e != nil {
				return e
			}
			_, _ = fmt.Fprintln(h, rel)
			f, e := os.Open(path)
			if e != nil {
				return e
			}
			_, e = io.Copy(h, f)
			ce := f.Close()
			if e != nil {
				return e
			}
			return ce
		})
		if e != nil {
			return "", e
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Stage runs and records a named project command, optionally reusing a verified result.
func (a *App) Stage(ctx context.Context, r Repo, w Workspace, name string, argv []string, o StageOptions) (Stage, error) {
	if e := CheckName(name); e != nil {
		return Stage{}, e
	}
	if len(argv) == 0 {
		return Stage{}, Fail("invalid_command", "Stage requires command argv")
	}
	if o.TimeoutSeconds < 0 || o.TimeoutSeconds > 3600 {
		return Stage{}, Fail("invalid_timeout", "Stage timeout must not exceed 3600 seconds")
	}
	if o.Reuse && len(o.Inputs) == 0 && len(o.Check) == 0 {
		return Stage{}, Fail("invalid_reuse", "Reusable stages require inputs or a live check")
	}
	if o.Cwd == "" {
		o.Cwd = "."
	}
	dir, e := relative(w.Path, o.Cwd)
	if e != nil {
		return Stage{}, e
	}
	info, e := os.Stat(dir)
	if e != nil {
		return Stage{}, e
	}
	if !info.IsDir() {
		return Stage{}, Fail("invalid_path", "Stage cwd must be a directory")
	}
	o.EnvUnset = append([]string{}, o.EnvUnset...)
	sort.Strings(o.EnvUnset)
	if _, e := childEnvironment(o.Env, o.EnvUnset, !o.NoInheritEnv); e != nil {
		return Stage{}, e
	}
	fp, e := fingerprint(ctx, w.Path, argv, o)
	if e != nil {
		return Stage{}, e
	}
	s, e := r.Load()
	if e != nil {
		return Stage{}, e
	}
	prior := s.Workspaces[w.ID].Stages[name]
	timeout := taskTimeout(o.TimeoutSeconds)
	execute := func(argv []string, quiet bool) error {
		_, err := a.RunCommand(ctx, dir, argv, CommandOptions{Env: o.Env, Unset: o.EnvUnset, NoInheritEnv: o.NoInheritEnv, Timeout: timeout, Capture: quiet})
		return err
	}
	if o.Reuse && prior.Status == "succeeded" && prior.Fingerprint == fp {
		if len(o.Check) == 0 || execute(o.Check, true) == nil {
			prior.Reused = true
			e = r.Update(ctx, w.ID, func(w *Workspace) { w.Stages[name] = prior })
			return prior, e
		}
	}
	st := Stage{Status: "running", Fingerprint: fp, StartedAt: time.Now().UTC()}
	if e = r.Update(ctx, w.ID, func(w *Workspace) { w.Stages[name] = st; w.LastError = "" }); e != nil {
		return st, e
	}
	_, _ = fmt.Fprintln(a.Out, "stage:", name)
	e = execute(argv, false)
	if e == nil && len(o.Check) > 0 {
		e = execute(o.Check, false)
	}
	now := time.Now().UTC()
	st.FinishedAt = &now
	st.Status = "succeeded"
	if e != nil {
		st.Status = "failed"
		st.Error = e.Error()
	}
	// Persist the failure even when the caller cancelled its execution context.
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	pe := r.Update(persist, w.ID, func(w *Workspace) {
		w.Stages[name] = st
		if e != nil {
			w.LastError = e.Error()
		}
	})
	if pe != nil {
		return st, pe
	}
	return st, e
}
