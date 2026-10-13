package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDoctorToolReadiness(t *testing.T) {
	for _, tc := range []struct {
		name, missing, failed, empty, want string
		optional                           bool
	}{
		{name: "ready", want: "gh_ready", optional: true},
		{name: "jj missing", missing: "jj", want: "jj_missing"},
		{name: "jj broken", failed: "--version", want: "jj_unavailable"},
		{name: "name missing", empty: "user.name", want: "jj_user_name_missing"},
		{name: "email unreadable", failed: "user.email", want: "jj_user_email_missing"},
		{name: "github missing", missing: "gh", want: "gh_missing", optional: true},
		{name: "github auth or network failure", failed: "auth", want: "gh_auth_unavailable", optional: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(_ context.Context, dir, name string) (string, error) {
				if dir != "/repo" {
					t.Fatalf("wrong cwd %q", dir)
				}
				if name == tc.missing {
					return "", os.ErrNotExist
				}
				return name, nil
			}
			probe := func(_ context.Context, dir, binary string, args ...string) (string, error) {
				if dir != "/repo" {
					t.Fatalf("wrong cwd %q", dir)
				}
				joined := strings.Join(args, " ")
				if tc.failed != "" && strings.Contains(joined, tc.failed) {
					return "secret-token", errors.New("secret-token")
				}
				if tc.empty != "" && strings.Contains(joined, tc.empty) {
					return "", nil
				}
				if binary == "gh" && joined != "auth status --active --hostname github.com" {
					t.Fatalf("unexpected auth probe %q", joined)
				}
				return "configured", nil
			}
			checks := doctorTools(context.Background(), "/repo", lookup, probe)
			found := false
			for _, check := range checks {
				if strings.Contains(check.Message+check.Remedy, "secret-token") {
					t.Fatal("child output leaked")
				}
				if check.Code == tc.want {
					found = true
					if check.Required == tc.optional {
						t.Fatalf("wrong requirement: %+v", check)
					}
				}
			}
			if !found {
				t.Fatalf("missing %s: %+v", tc.want, checks)
			}
		})
	}
}

func TestDoctorMissingJJStillReportsOptionalTools(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	h, err := New(t.TempDir(), t.TempDir(), nil).Doctor(context.Background())
	if ErrorCode(err) != "unhealthy_prerequisites" || len(h.Tools) != 3 || h.Tools[0].Code != "jj_missing" || h.Tools[1].Code != "gh_missing" {
		t.Fatalf("missing prerequisites lost: %+v %v", h, err)
	}
}

func TestDoctorOutsideWorkspaceStillReportsTools(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "jj"), []byte("#!/bin/sh\ncase \"$1\" in\n--version|config) printf configured;;\n*) exit 1;;\nesac\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	h, err := New(t.TempDir(), t.TempDir(), nil).Doctor(context.Background())
	if ErrorCode(err) != "not_a_jj_repository" || len(h.Tools) != 3 || h.Tools[1].Code != "gh_missing" {
		t.Fatalf("discovery lost diagnostics: %+v %v", h, err)
	}
}

func TestDoctorProbeCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := probeDoctor(ctx, t.TempDir(), "/bin/sh", "-c", "exit 0")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestDoctorProbeDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := probeDoctor(ctx, t.TempDir(), "/bin/sh", "-c", "while :; do :; done")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline lost: %v", err)
	}
}

func TestDoctorOptionalAuthFailurePreservesWorkspaceHealth(t *testing.T) {
	a, r, _ := fixture(t)
	jj, err := exec.LookPath("jj")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.Symlink(jj, filepath.Join(bin, "jj")); err != nil {
		t.Fatal(err)
	}
	// User-installed JJ wrappers may use /usr/bin/env bash.
	if bash, err := exec.LookPath("bash"); err == nil {
		if err := os.Symlink(bash, filepath.Join(bin, "bash")); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nprintf secret-token\nprintf secret-token >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	statePath := filepath.Join(r.Store, "state.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	h, err := a.Doctor(context.Background())
	if err != nil || len(h.Issues) != 0 || len(h.Tools) != 3 || h.Tools[1].Code != "gh_auth_unavailable" {
		t.Fatalf("optional auth blocked workspace: %+v %v", h, err)
	}
	after, err := os.ReadFile(statePath)
	if err != nil || string(before) != string(after) {
		t.Fatalf("doctor changed registry: %v", err)
	}
}

func isolateDoctorTools(t *testing.T, a *App) {
	t.Helper()
	jj, err := exec.LookPath("jj")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.Symlink(jj, filepath.Join(bin, "jj")); err != nil {
		t.Fatal(err)
	}
	if bash, err := exec.LookPath("bash"); err == nil {
		if err := os.Symlink(bash, filepath.Join(bin, "bash")); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	a.Reviews = fakeReviewClient{diagnose: func(context.Context, string) ReviewResolution {
		return ReviewResolution{Status: "ready", Code: "linear_ready"}
	}}
}

func TestDoctorValidatesBeforeRegistrationWithoutExecutingLuaOrMutatingJJ(t *testing.T) {
	a, r, anchor := fixture(t)
	config(t, anchor.Path, `error("doctor must not execute this module")`)
	if err := os.Remove(filepath.Join(r.Store, "state.json")); err != nil {
		t.Fatal(err)
	}
	isolateDoctorTools(t, a)
	before, err := output(t.Context(), anchor.Path, "op", "log", "--limit", "1", "--no-graph", "-T", "id")
	if err != nil {
		t.Fatal(err)
	}
	health, err := a.Doctor(t.Context())
	if err != nil || health.Configuration.Status != "ready" || health.WorkspaceCheck.Status != "unregistered" || len(health.Tools) != 3 || health.Workspace.ID != "" {
		t.Fatalf("unregistered diagnostics failed: %+v %v", health, err)
	}
	if _, err := os.Stat(filepath.Join(r.Store, "state.json")); !os.IsNotExist(err) {
		t.Fatalf("doctor registered state: %v", err)
	}
	after, err := output(t.Context(), anchor.Path, "op", "log", "--limit", "1", "--no-graph", "-T", "id")
	if err != nil || before != after {
		t.Fatalf("doctor mutated JJ: %s -> %s %v", before, after, err)
	}
}

func TestDoctorReportsConfigurationToolsAndWorkspaceFailuresTogether(t *testing.T) {
	a, r, anchor := fixture(t)
	config(t, anchor.Path, `return {broken syntax`)
	if err := r.Update(t.Context(), anchor.ID, func(w *Workspace) { w.Stages["prepare"] = Stage{Status: "failed"}; w.LastError = "Preparation failed" }); err != nil {
		t.Fatal(err)
	}
	isolateDoctorTools(t, a)
	before, err := os.ReadFile(filepath.Join(r.Store, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	health, err := a.Doctor(t.Context())
	if ErrorCode(err) != "invalid_hook" || health.Configuration.Status != "failed" || health.WorkspaceCheck.Status != "failed" || health.Workspace.ID != anchor.ID || len(health.Issues) < 3 || len(health.Tools) != 3 {
		t.Fatalf("checks hid another failure: %+v %v", health, err)
	}
	after, err := os.ReadFile(filepath.Join(r.Store, "state.json"))
	if err != nil || string(before) != string(after) {
		t.Fatalf("doctor mutated state: %v", err)
	}
}

func TestDoctorMissingJJStillValidatesRootConfigurationFromSubdirectory(t *testing.T) {
	a, _, anchor := fixture(t)
	config(t, anchor.Path, `return {}`)
	if err := os.WriteFile(filepath.Join(anchor.Path, ".wtf", "config.toml"), []byte("version = ["), 0600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(anchor.Path, "sub")
	if err := os.Mkdir(sub, 0700); err != nil {
		t.Fatal(err)
	}
	a.Cwd = sub
	t.Setenv("PATH", t.TempDir())
	health, err := a.Doctor(t.Context())
	if ErrorCode(err) != "unhealthy_prerequisites" || health.Configuration.Code != "invalid_config" || health.Configuration.Status != "failed" || health.Tools[0].Code != "jj_missing" || health.WorkspaceCheck.Status != "unavailable" {
		t.Fatalf("missing JJ hid configuration: %+v %v", health, err)
	}
}

func TestDoctorDistinguishesMissingAndMalformedHookConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name, code, status, message string
		directory                   bool
		files                       map[string]string
		fails                       bool
	}{
		{name: "no directory", code: "wtf_directory_missing", status: "not_configured", message: "No .wtf directory found"},
		{name: "no TOML", directory: true, code: "configuration_missing", status: "not_configured", message: "No .wtf/config.toml found"},
		{name: "no Lua", directory: true, files: map[string]string{"config.toml": "version=1\n"}, code: "hooks_not_configured", status: "ready", message: "no Lua hook entrypoint configured"},
		{name: "malformed TOML", directory: true, files: map[string]string{"config.toml": "version=["}, code: "invalid_config", status: "failed", fails: true},
		{name: "missing Lua file", directory: true, files: map[string]string{"config.toml": "version=1\nlua='missing.lua'\n"}, code: "hook_missing", status: "failed", message: "missing.lua", fails: true},
		{name: "malformed Lua", directory: true, files: map[string]string{"config.toml": "version=1\nlua='hooks.lua'\n", "hooks.lua": "return {broken syntax"}, code: "invalid_hook", status: "failed", fails: true},
		{name: "valid Lua", directory: true, files: map[string]string{"config.toml": "version=1\nlua='hooks.lua'\n", "hooks.lua": `error("must not execute")`}, code: "configuration_valid", status: "ready", message: "Lua entrypoint syntax are valid"},
		{name: "creation hooks", directory: true, files: map[string]string{"config.toml": "version=1\n[new]\nhooks=['hello']\n", "hooks/hello.lua": `error("must not execute")`}, code: "configuration_valid", status: "ready", message: "selected Lua hook syntax are valid"},
		{name: "invocation hooks", directory: true, files: map[string]string{"config.toml": "version=1\n[invocation]\nbefore=['hello']\n", "hooks/hello.lua": `error("must not execute")`}, code: "configuration_valid", status: "ready", message: "selected Lua hook syntax are valid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _, anchor := fixture(t)
			if tc.directory {
				if err := os.Mkdir(filepath.Join(anchor.Path, ".wtf"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			for path, content := range tc.files {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(anchor.Path, ".wtf", path)), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(anchor.Path, ".wtf", path), []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			isolateDoctorTools(t, a)
			health, err := a.Doctor(t.Context())
			if (err != nil) != tc.fails || health.Configuration.Code != tc.code || health.Configuration.Status != tc.status || !strings.Contains(health.Configuration.Message, tc.message) {
				t.Fatalf("wrong configuration report: %+v %v", health, err)
			}
			if tc.code != "configuration_valid" && strings.Contains(health.Configuration.Message, "Lua entrypoint syntax are valid") {
				t.Fatal("claimed to validate absent or malformed Lua")
			}
		})
	}
}
