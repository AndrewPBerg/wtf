package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) create(ctx context.Context, r Repo, path, name, from string, options NewWorkspaceOptions) (Workspace, error) {
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
	native, e := NativeWorkspaces(ctx, r.Root)
	if e != nil {
		return Workspace{}, e
	}
	if _, exists := native[name]; exists {
		return Workspace{}, Fail("workspace_exists", "JJ workspace "+name+" already exists; choose a different destination basename")
	}
	now := time.Now().UTC()
	w := Workspace{ID: id, Name: name, Path: path, Owned: true, Status: "creating", Stages: map[string]Stage{}, CreatedAt: &now}
	if options.Message != nil {
		w.Message = *options.Message
	}
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
	e = inheritWorkspaceConfig(ctx, r.Root, path)
	if e == nil {
		e = writeAtomic(workspaceMarkerPath(path), map[string]string{"id": id, "repository_id": s.RepositoryID})
	}
	if e != nil {
		p, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = r.Update(p, id, func(w *Workspace) { w.Status = "creation_failed"; w.LastError = e.Error() })
		return w, e
	}
	w.Status = "ready"
	e = r.Update(ctx, id, func(old *Workspace) { old.Status = "ready" })
	return w, e
}

// NewWorkspace prepares an existing workspace root or creates one at the exact
// destination path. Bare names resolve beside the registered repository anchor;
// explicit relative paths resolve from the invoking directory.
func (a *App) NewWorkspace(ctx context.Context, path, from string, args ...string) (Initialization, error) {
	return a.NewWorkspaceWithOptions(ctx, path, from, NewWorkspaceOptions{}, args...)
}

// NewWorkspaceOptions contains optional metadata changes. A nil message preserves
// an existing description; an explicitly empty message clears it.
type NewWorkspaceOptions struct {
	Message *string
}

// NewWorkspaceWithOptions creates or prepares a workspace with optional metadata.
func (a *App) NewWorkspaceWithOptions(ctx context.Context, path, from string, options NewWorkspaceOptions, args ...string) (Initialization, error) {
	if options.Message != nil && strings.ContainsRune(*options.Message, 0) {
		return Initialization{}, Fail("invalid_message", "Workspace messages cannot contain NUL characters")
	}
	if path == "" || strings.ContainsRune(path, 0) {
		return Initialization{}, Fail("invalid_path", "A workspace destination path is required")
	}
	path, err := a.ResolveWorkspacePath(ctx, path)
	if err != nil {
		return Initialization{}, err
	}
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return Initialization{}, Fail("path_exists", "Destination exists and is not a JJ workspace directory")
		}
		r, err := Discover(ctx, path)
		if err != nil {
			return Initialization{}, err
		}
		if !samePath(path, r.Root) {
			return Initialization{}, Fail("not_workspace_root", "Destination must be a JJ workspace root, not a directory inside one")
		}
		target := *a
		target.Cwd = r.Root
		w, err := r.Workspace(ctx)
		if err != nil && ErrorCode(err) != "not_registered" && ErrorCode(err) != "unmanaged_workspace" {
			return Initialization{}, err
		}
		return target.initializeOperation(ctx, r, w, false, options, args...)
	}
	if !os.IsNotExist(err) {
		return Initialization{}, err
	}
	// A dangling symlink is an occupied destination too.
	if _, err := os.Lstat(path); err == nil || !os.IsNotExist(err) {
		return Initialization{}, Fail("path_exists", "Destination already exists or cannot be inspected")
	}
	w, err := a.createWorkspace(ctx, path, from, options, args...)
	return Initialization{Workspace: w, Code: "initialized"}, err
}

func (a *App) createWorkspace(ctx context.Context, path, from string, options NewWorkspaceOptions, args ...string) (Workspace, error) {
	name := filepath.Base(path)
	if err := CheckName(name); err != nil {
		return Workspace{}, err
	}
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
	rt.Destination = path
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
		w, e = a.create(ctx, r, path, name, from, options)
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
		initialized, err := child.initializeOperation(rt.Context, Repo{w.Path, r.Store}, w, true, options, args...)
		w, e = initialized.Workspace, err
		rt.Bind(w)
		return w, e
	}
	e = rt.Override("new", name, from, defaultFn, args...)
	if e == nil && created {
		e = rt.runCreationHooks(w.Name, from, args)
	}
	if e != nil && w.ID != "" {
		p, c := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer c()
		_ = r.Update(p, w.ID, func(w *Workspace) { w.Status = "hook_failed"; w.LastError = e.Error() })
		if current, err := r.Select(w.ID); err == nil {
			w = current
		}
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
func (a *App) initializeOperation(ctx context.Context, r Repo, w Workspace, locked bool, options NewWorkspaceOptions, args ...string) (Initialization, error) {
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
			if options.Message != nil {
				if err := r.Update(rt.Context, w.ID, func(w *Workspace) { w.Message = *options.Message }); err != nil {
					return w, err
				}
				w.Message = *options.Message
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
	return a.RemoveWithOptions(ctx, name, RemovalOptions{})
}

// RemovalOptions controls whether remaining source changes may be discarded.
type RemovalOptions struct {
	Force bool
	proof *removalProof
}

// RemoveWithOptions retains identity, ownership, and lifecycle checks in force mode.
func (a *App) RemoveWithOptions(ctx context.Context, name string, options RemovalOptions) (Workspace, error) {
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
	if !options.Force && options.proof == nil {
		bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
		assessment := a.removalWithPRs(bounded, r, w)
		cancel()
		if !assessment.CanRemove(false) {
			code := assessment.Code
			if code == "" {
				code = "inspection_failed"
			}
			return w, Fail(code, assessment.Warning)
		}
		options.proof = assessment.proof
	}
	unlock, e := r.LockWorkspace(ctx, w.ID)
	if e != nil {
		return w, e
	}
	defer unlock()
	if e = a.checkRemovalChanges(ctx, w, options); e != nil {
		return w, e
	}
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
		if e = a.checkRemovalChanges(ctx, w, options); e != nil {
			return w, e
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
	e = readJSON(workspaceMarkerPath(w.Path), &marker)
	if os.IsNotExist(e) {
		// Existing workspaces retain their original identity until migrated.
		e = readJSON(filepath.Join(w.Path, ".wtf", "workspace.json"), &marker)
	}
	if e != nil {
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

// JJ excludes its workspace metadata directory independently of source ignores.
func workspaceMarkerPath(root string) string {
	return filepath.Join(root, ".jj", "wtf-workspace.json")
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

// DiagnosticCheck reports one independently inspected area.
type DiagnosticCheck struct {
	Code    string `json:"code"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Remedy  string `json:"remedy,omitempty"`
}

// Health contains independent configuration, workspace and tool diagnostics.
type Health struct {
	Workspace      Workspace       `json:"workspace"`
	Claims         []Claim         `json:"claims"`
	Issues         []string        `json:"issues"`
	Tools          []ToolCheck     `json:"tools"`
	Configuration  DiagnosticCheck `json:"configuration"`
	WorkspaceCheck DiagnosticCheck `json:"workspace_check"`
}

// Doctor checks tool readiness and current workspace health without repairing it.
func (a *App) Doctor(ctx context.Context, configurationErrors ...error) (Health, error) {
	h := Health{Claims: []Claim{}, Issues: []string{}, Tools: doctorTools(ctx, a.Cwd, doctorBinary, probeDoctor)}
	h.Tools = append(h.Tools, a.linearToolCheck(ctx))
	var first error
	record := func(err error) {
		if err != nil {
			h.Issues = append(h.Issues, err.Error())
			if first == nil {
				first = err
			}
		}
	}
	for _, check := range h.Tools {
		if check.Required && check.Status != "ready" {
			record(Fail("unhealthy_prerequisites", check.Message))
		}
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, discoveryErr := Discover(bounded, a.Cwd)
	root := doctorConfigRoot(a.Cwd)
	if discoveryErr == nil {
		root = r.Root
	}
	h.Configuration = DiagnosticCheck{Code: "configuration_valid", Status: "ready", Message: "Configuration and Lua entrypoint syntax are valid"}
	configuration, configErr := LoadConfig(root)
	if configErr == nil && len(configuration.hookModules()) > 0 {
		h.Configuration.Message = "Configuration and selected Lua hook syntax are valid"
	}
	if configErr == nil && configuration.Lua == "" && len(configuration.hookModules()) == 0 {
		h.Configuration = DiagnosticCheck{Code: "hooks_not_configured", Status: "ready", Message: "TOML configuration is valid; no Lua hook entrypoint configured"}
		directory := filepath.Join(root, ".wtf")
		if _, err := os.Stat(directory); os.IsNotExist(err) {
			h.Configuration = DiagnosticCheck{Code: "wtf_directory_missing", Status: "not_configured", Message: "No .wtf directory found; no configuration found for hooks"}
		} else if _, err := os.Stat(filepath.Join(directory, "config.toml")); os.IsNotExist(err) {
			h.Configuration = DiagnosticCheck{Code: "configuration_missing", Status: "not_configured", Message: "No .wtf/config.toml found; no configuration found for hooks"}
		}
	}
	for _, err := range append([]error{configErr}, configurationErrors...) {
		if err != nil {
			if h.Configuration.Status != "failed" {
				h.Configuration.Code = ErrorCode(err)
				h.Configuration.Message = err.Error()
			} else {
				h.Configuration.Message += "; " + err.Error()
			}
			h.Configuration.Status, h.Configuration.Remedy = "failed", "Fix the reported TOML settings or Lua entrypoint and run wtf doctor again"
			record(err)
		}
	}
	h.WorkspaceCheck = DiagnosticCheck{Code: "workspace_ready", Status: "ready", Message: "Workspace identity and retained state are healthy"}
	if discoveryErr != nil {
		h.WorkspaceCheck = DiagnosticCheck{Code: ErrorCode(discoveryErr), Status: "unavailable", Message: discoveryErr.Error()}
		record(discoveryErr)
	} else {
		w, err := r.Workspace(bounded)
		if err != nil {
			h.WorkspaceCheck = DiagnosticCheck{Code: ErrorCode(err), Status: "failed", Message: err.Error()}
			if ErrorCode(err) == "not_registered" || ErrorCode(err) == "unmanaged_workspace" {
				h.WorkspaceCheck.Status, h.WorkspaceCheck.Remedy = "unregistered", "Run wtf new . to register and prepare this workspace"
			} else {
				record(err)
			}
		} else {
			h.Workspace = w
			start := len(h.Issues)
			claims, err := r.Claims(w)
			if err == nil {
				h.Claims = claims
			} else {
				record(err)
			}
			if w.Owned {
				record(verifyIdentity(bounded, r, w))
			}
			for name, stage := range w.Stages {
				if stage.Status != "succeeded" {
					record(Fail("unhealthy_workspace", fmt.Sprintf("stage %s is %s", name, stage.Status)))
				}
			}
			if w.LastError != "" {
				record(Fail("unhealthy_workspace", w.LastError))
			}
			if len(h.Issues) > start {
				h.WorkspaceCheck.Code, h.WorkspaceCheck.Status, h.WorkspaceCheck.Message = "unhealthy_workspace", "failed", strings.Join(h.Issues[start:], "; ")
			}
		}
	}
	if first != nil {
		return h, Fail(ErrorCode(first), strings.Join(h.Issues, "; "))
	}
	return h, nil
}
