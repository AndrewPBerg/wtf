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
