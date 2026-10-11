package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) create(ctx context.Context, r Repo, name, from string) (Workspace, error) {
	if e := CheckName(name); e != nil {
		return Workspace{}, e
	}
	if from == "" {
		from = "trunk()"
	}
	s, e := r.Load()
	if e != nil {
		return Workspace{}, e
	}
	id, e := ID()
	if e != nil {
		return Workspace{}, e
	}
	path := filepath.Join(filepath.Dir(s.Anchor), name+"--"+filepath.Base(s.Anchor))
	w := Workspace{id, name, path, true, "creating", map[string]Stage{}, ""}
	e = r.Edit(ctx, func(s *State) error {
		for _, old := range s.Workspaces {
			if old.Status != "removed" && (old.Name == name || old.Path == path) {
				return Fail("workspace_exists", "Workspace "+name+" already exists")
			}
		}
		if _, e := os.Lstat(path); !os.IsNotExist(e) {
			return Fail("path_exists", "Destination already exists or cannot be inspected")
		}
		s.Workspaces[id] = w
		return nil
	})
	if e != nil {
		return Workspace{}, e
	}
	_, e = output(ctx, r.Root, "workspace", "add", "--name", name, "-r", from, path)
	if e != nil {
		p, c := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer c()
		_ = r.Update(p, id, func(w *Workspace) { w.Status = "creation_failed"; w.LastError = e.Error() })
		return w, e
	}
	if e = os.MkdirAll(filepath.Join(path, ".wtf"), 0700); e == nil {
		e = writeAtomic(filepath.Join(path, ".wtf", "workspace.json"), map[string]string{"id": id, "repository_id": s.RepositoryID})
	}
	if e != nil {
		_ = r.Update(ctx, id, func(w *Workspace) { w.Status = "creation_failed"; w.LastError = e.Error() })
		return w, e
	}
	w.Status = "ready"
	e = r.Update(ctx, id, func(old *Workspace) { old.Status = "ready" })
	return w, e
}

// NewWorkspace creates a JJ workspace and runs shared initialization hooks.
func (a *App) NewWorkspace(ctx context.Context, name, from string, args ...string) (Workspace, error) {
	r, e := Discover(ctx, a.Cwd)
	if e != nil {
		return Workspace{}, e
	}
	if _, e = r.Load(); e != nil {
		return Workspace{}, e
	}
	rt, e := a.runtime(ctx, r, Workspace{})
	if e != nil {
		return Workspace{}, e
	}
	defer rt.Close()
	var w Workspace
	var unlock func()
	created := false
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	defaultFn := func(name, from string) (Workspace, error) {
		if created {
			return w, Fail("default_already_called", "Default creation can only be called once per operation")
		}
		var e error
		w, e = a.create(ctx, r, name, from)
		if e != nil {
			return w, e
		}
		created = true
		unlock, e = r.LockWorkspace(ctx, w.ID)
		if e != nil {
			return w, e
		}
		child := *a
		child.Cwd = w.Path
		initialized, err := child.initializeOperation(rt.Context, Repo{w.Path, r.Store}, w, true, args...)
		w, e = initialized.Workspace, err
		rt.Bind(w)
		return w, e
	}
	e = rt.Override("new", name, from, defaultFn, args...)
	if e != nil && w.ID != "" {
		p, c := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer c()
		_ = r.Update(p, w.ID, func(w *Workspace) { w.Status = "hook_failed"; w.LastError = e.Error() })
	}
	if e == nil && !created {
		return w, Fail("invalid_override", "Pilot new overrides must delegate to default creation")
	}
	if e == nil {
		if e = r.Update(ctx, w.ID, func(w *Workspace) { w.Status = "ready"; w.LastError = "" }); e != nil {
			return w, e
		}
		w, e = r.Select(w.ID)
	}
	return w, e
}

// New already owns the workspace lock; direct init acquires it once identity is known.
// Both operations dispatch the init override, whose default registers that identity.
func (a *App) initializeOperation(ctx context.Context, r Repo, w Workspace, locked bool, args ...string) (Initialization, error) {
	code := "initialized"
	if w.ID != "" {
		code = "already_initialized"
	}
	var unlock func()
	defer func() {
		if unlock != nil {
			unlock()
		}
	}()
	if w.ID != "" && !locked {
		var err error
		unlock, err = r.LockWorkspace(ctx, w.ID)
		if err != nil {
			return Initialization{Workspace: w}, err
		}
		locked = true
	}
	name := w.Name
	if name == "" {
		native, err := NativeWorkspaces(ctx, r.Root)
		if err != nil {
			return Initialization{}, err
		}
		for candidate, path := range native {
			if samePath(path, r.Root) {
				name = candidate
				break
			}
		}
		if name == "" {
			return Initialization{}, Fail("jj_failed", "Cannot identify current native workspace")
		}
	}
	rt, err := a.runtime(ctx, r, w)
	if err == nil {
		defer rt.Close()
		called, delegated := false, false
		err = rt.Override("init", name, "", func(_, _ string) (Workspace, error) {
			if called {
				return w, Fail("default_already_called", "Default initialization can only be called once per operation")
			}
			called = true
			var err error
			w, err = a.initialize(rt.Context, &code)
			if err != nil {
				return w, err
			}
			if !locked {
				unlock, err = r.LockWorkspace(rt.Context, w.ID)
				if err != nil {
					return w, err
				}
				locked = true
			}
			rt.Bind(w)
			delegated = true
			return w, nil
		}, args...)
		if err == nil && !delegated {
			err = Fail("invalid_override", "Init overrides must delegate to default registration")
		}
	}
	if w.ID == "" {
		return Initialization{}, err
	}
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if saved := r.Update(persist, w.ID, func(w *Workspace) {
		if err != nil {
			w.Status, w.LastError = "hook_failed", err.Error()
		} else {
			w.Status, w.LastError = "ready", ""
		}
	}); saved != nil {
		return Initialization{Workspace: w, Code: code}, saved
	}
	updated, saved := r.Select(w.ID)
	if saved != nil {
		return Initialization{Workspace: w, Code: code}, saved
	}
	return Initialization{Workspace: updated, Code: code}, err
}

// Remove tears down and removes an owned workspace after safety checks.
func (a *App) Remove(ctx context.Context, name string) (Workspace, error) {
	r, e := Discover(ctx, a.Cwd)
	if e != nil {
		return Workspace{}, e
	}
	w, e := r.Select(name)
	if e != nil {
		return w, e
	}
	if !w.Owned {
		return w, Fail("protected_workspace", "Adopted and anchor workspaces cannot be removed by this pilot")
	}
	if samePath(w.Path, r.Root) {
		return w, Fail("current_workspace", "Switch out of the target workspace before removal")
	}
	if e = verifyIdentity(ctx, r, w); e != nil {
		return w, e
	}
	unlock, e := r.LockWorkspace(ctx, w.ID)
	if e != nil {
		return w, e
	}
	defer unlock()
	target := Repo{w.Path, r.Store}
	rt, e := a.runtime(ctx, target, w)
	if e != nil {
		return w, e
	}
	defer rt.Close()
	removed := false
	e = rt.Override("remove", w.Name, "", func(string, string) (Workspace, error) {
		if removed {
			return w, Fail("default_already_called", "Removal already completed")
		}
		if e = verifyIdentity(ctx, r, w); e != nil {
			return w, e
		}
		claims, e := r.Claims(w)
		if e != nil {
			return w, e
		}
		for _, claim := range claims {
			if claim.Kind != "port" {
				return w, Fail("cleanup_required", "Teardown must release external resource claims before removal")
			}
		}
		if _, e = output(ctx, w.Path, "status"); e != nil {
			return w, e
		}
		diff, e := output(ctx, w.Path, "diff", "--summary")
		if e != nil {
			return w, e
		}
		if diff != "" {
			return w, Fail("workspace_has_changes", "Workspace contains changes; preserve them before removal")
		}
		if _, e = output(ctx, r.Root, "workspace", "forget", w.Name); e != nil {
			return w, e
		}
		if e = os.RemoveAll(w.Path); e != nil {
			return w, e
		}
		e = r.Edit(ctx, func(s *State) error {
			for k, c := range s.Claims {
				if c.Owner == w.ID {
					delete(s.Claims, k)
				}
			}
			old := s.Workspaces[w.ID]
			old.Status = "removed"
			old.LastError = ""
			s.Workspaces[w.ID] = old
			w = old
			return nil
		})
		removed = e == nil
		return w, e
	})
	if e == nil && !removed {
		e = Fail("invalid_override", "Pilot remove overrides must delegate to default removal")
	}
	if e != nil {
		p, c := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer c()
		_ = r.Update(p, w.ID, func(w *Workspace) { w.Status = "cleanup_failed"; w.LastError = e.Error() })
	}
	return w, e
}
func verifyIdentity(ctx context.Context, r Repo, w Workspace) error {
	native, e := NativeWorkspaces(ctx, r.Root)
	if e != nil {
		return e
	}
	p, ok := native[w.Name]
	if !ok || !samePath(p, w.Path) {
		return Fail("identity_mismatch", "Native JJ workspace path does not match the managed identity")
	}
	var marker map[string]string
	if e = readJSON(filepath.Join(w.Path, ".wtf", "workspace.json"), &marker); e != nil {
		return Fail("identity_mismatch", "Workspace marker is missing or unreadable")
	}
	s, e := r.Load()
	if e != nil {
		return e
	}
	if marker["id"] != w.ID || marker["repository_id"] != s.RepositoryID {
		return Fail("identity_mismatch", "Workspace marker belongs to a different identity")
	}
	return nil
}

// Hook runs a named hook under the current workspace lock.
func (a *App) Hook(ctx context.Context, name string, args ...string) (Workspace, error) {
	r, e := Discover(ctx, a.Cwd)
	if e != nil {
		return Workspace{}, e
	}
	w, e := r.Workspace(ctx)
	if e != nil {
		return w, e
	}
	unlock, e := r.LockWorkspace(ctx, w.ID)
	if e != nil {
		return w, e
	}
	defer unlock()
	rt, e := a.runtime(ctx, r, w)
	if e != nil {
		return w, e
	}
	defer rt.Close()
	e = rt.Run(name, args...)
	if e != nil {
		p, c := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer c()
		_ = r.Update(p, w.ID, func(w *Workspace) { w.Status = "hook_failed"; w.LastError = e.Error() })
		return w, e
	}
	e = r.Update(ctx, w.ID, func(w *Workspace) { w.Status = "ready"; w.LastError = "" })
	if e != nil {
		return w, e
	}
	return r.Select(w.ID)
}

// Health contains workspace stages, resource claims and tool diagnostics.
type Health struct {
	Workspace Workspace   `json:"workspace"`
	Claims    []Claim     `json:"claims"`
	Issues    []string    `json:"issues"`
	Tools     []ToolCheck `json:"tools"`
}

// Doctor checks tool readiness and current workspace health without repairing it.
func (a *App) Doctor(ctx context.Context) (Health, error) {
	h := Health{Claims: []Claim{}, Issues: []string{}, Tools: doctorTools(ctx, a.Cwd, doctorBinary, probeDoctor)}
	h.Tools = append(h.Tools, a.linearToolCheck(ctx))
	for _, check := range h.Tools {
		if check.Required && check.Status != "ready" {
			h.Issues = append(h.Issues, check.Message)
		}
	}
	if len(h.Issues) > 0 {
		return h, Fail("unhealthy_prerequisites", strings.Join(h.Issues, "; "))
	}
	r, e := Discover(ctx, a.Cwd)
	if e != nil {
		return h, e
	}
	w, e := r.Workspace(ctx)
	if e != nil {
		return h, e
	}
	claims, e := r.Claims(w)
	if e != nil {
		return h, e
	}
	h.Workspace, h.Claims = w, claims
	if _, e = LoadConfig(w.Path); e != nil {
		h.Issues = append(h.Issues, e.Error())
	}
	if w.Owned {
		if e = verifyIdentity(ctx, r, w); e != nil {
			h.Issues = append(h.Issues, e.Error())
		}
	}
	for name, st := range w.Stages {
		if st.Status != "succeeded" {
			h.Issues = append(h.Issues, fmt.Sprintf("stage %s is %s", name, st.Status))
		}
	}
	if w.LastError != "" {
		h.Issues = append(h.Issues, w.LastError)
	}
	if len(h.Issues) > 0 {
		return h, Fail("unhealthy_workspace", strings.Join(h.Issues, "; "))
	}
	return h, nil
}
