package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
)

func TestDoctorJSONAndHumanShowMissingPrerequisites(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	a := core.New(t.TempDir(), t.TempDir(), nil)
	result, code, _ := invoke(t, a, "DoCtOr", "-J")
	if code == 0 || result.Error == nil || result.Error.Code != "unhealthy_prerequisites" || result.Data == nil {
		t.Fatalf("missing structured diagnostics: %+v", result)
	}
	var out bytes.Buffer
	s := New(a, &out, &out)
	if s.Execute(context.Background(), []string{"doctor"}) == 0 {
		t.Fatal("missing JJ succeeded")
	}
	for _, want := range []string{"jj", "gh", "Install Jujutsu", "Install GitHub CLI", "optional"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, &out)
		}
	}
}

func TestDocAliasUsesDoctorDiagnostics(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	a := core.New(t.TempDir(), t.TempDir(), nil)
	for _, alias := range []string{"doc", "DoC", "DOC"} {
		result, code, _ := invoke(t, a, alias, "-J")
		if code == 0 || result.Command != "wtf doctor" || result.Error == nil || result.Error.Code != "unhealthy_prerequisites" || result.Data == nil {
			t.Fatalf("alias did not run doctor diagnostics: %s %+v", alias, result)
		}
	}
}

func TestDoctorBrokenDefaultsStillReportsAllChecks(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	a := core.New(t.TempDir(), t.TempDir(), nil)
	if err := os.Mkdir(filepath.Join(a.Cwd, ".wtf"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.Cwd, ".wtf", "config.toml"), []byte("version = ["), 0600); err != nil {
		t.Fatal(err)
	}
	result, code, _ := invoke(t, a, "doctor", "-j")
	if code == 0 || result.Error == nil {
		t.Fatalf("invalid config succeeded: %+v", result)
	}
	encoded, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatal(err)
	}
	var health core.Health
	if err := json.Unmarshal(encoded, &health); err != nil {
		t.Fatal(err)
	}
	if health.Configuration.Status != "failed" || health.WorkspaceCheck.Status != "unavailable" || len(health.Tools) != 3 {
		t.Fatalf("defaults blocked diagnostics: %+v", result)
	}
	var out bytes.Buffer
	s := New(a, &out, &out)
	if s.Execute(t.Context(), []string{"doctor"}) == 0 {
		t.Fatal("invalid config succeeded")
	}
	for _, text := range []string{"Configuration", "Workspace", "Tools", "jj", "gh"} {
		if !strings.Contains(out.String(), text) {
			t.Fatalf("human report omitted %s: %s", text, &out)
		}
	}
}

func TestValidateIsNotACommand(t *testing.T) {
	a := core.New(t.TempDir(), t.TempDir(), nil)
	root := New(a, &bytes.Buffer{}, &bytes.Buffer{}).Root
	for _, command := range root.Commands() {
		if commandMatches(command, "validate") {
			t.Fatal("validate remains in command tree")
		}
	}
	result, code, _ := invoke(t, a, "VaLiDaTe", "-J")
	if code == 0 || result.OK || result.Error == nil || result.Error.Code != "invalid_arguments" {
		t.Fatalf("removed command succeeded: %+v", result)
	}
}

func TestDoctorHumanShowsOptionalWarningOnHealthyWorkspace(t *testing.T) {
	var out bytes.Buffer
	s := &Session{Data: core.Health{Workspace: core.Workspace{Name: "demo"}, Tools: []core.ToolCheck{{Tool: "gh", Status: "missing", Feature: "GitHub PR and check metadata", Message: "GitHub CLI is missing", Remedy: "Install GitHub CLI"}}}}
	if err := s.human(&out, nil); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Workspace healthy", "optional", "Install GitHub CLI"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, &out)
		}
	}
}

func TestDoctorHumanMissingHooksIsInformational(t *testing.T) {
	var out bytes.Buffer
	s := &Session{Data: core.Health{Configuration: core.DiagnosticCheck{Code: "wtf_directory_missing", Status: "not_configured", Message: "No .wtf directory found; no configuration found for hooks"}}}
	if err := s.human(&out, nil); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "No .wtf directory found") || strings.Contains(text, "Lua entrypoint syntax are valid") || strings.Contains(text, "! Configuration") || strings.Contains(text, "✓ Configuration") {
		t.Fatalf("missing hooks misrepresented: %s", text)
	}
}
