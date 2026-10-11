package cli

import (
	"bytes"
	"context"
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
