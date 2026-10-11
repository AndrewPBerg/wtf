package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/spf13/cobra"
)

func writeDefaults(t *testing.T, path, body string) {
	t.Helper()
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, []byte("version=1\n"+body), 0600); e != nil {
		t.Fatal(e)
	}
}
func TestCommandDefaultsLayerGlobalParentLeafAndExplicitFlags(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	outer := filepath.Join(base, "work")
	repo := filepath.Join(outer, "repo")
	leaf := filepath.Join(repo, "src")
	if e := os.MkdirAll(leaf, 0700); e != nil {
		t.Fatal(e)
	}
	writeDefaults(t, filepath.Join(home, "config.toml"), "[commands.LS]\npr=true\nhook=true\n")
	writeDefaults(t, filepath.Join(outer, ".wtf", "config.toml"), "[commands.list]\nhook=false\n")
	writeDefaults(t, filepath.Join(repo, ".wtf", "config.toml"), "[commands.list]\npr=false\n")
	writeDefaults(t, filepath.Join(leaf, ".wtf", "config.toml"), "[commands.LIST]\nPR=true\n")
	var out, logs bytes.Buffer
	s := New(core.New(leaf, home, &logs), &out, &logs)
	cmd, _, e := s.Root.Find([]string{"list"})
	if e != nil {
		t.Fatal(e)
	}
	cmd.SetContext(context.Background())
	if e = s.applyDefaults(cmd); e != nil {
		t.Fatal(e)
	}
	if !s.ShowPRs || s.ShowStages {
		t.Fatalf("wrong precedence PR=%v stages=%v", s.ShowPRs, s.ShowStages)
	}
	s = New(core.New(leaf, home, &logs), &out, &logs)
	cmd, _, e = s.Root.Find([]string{"list"})
	if e != nil {
		t.Fatal(e)
	}
	cmd.SetContext(context.Background())
	if e = cmd.Flags().Set("pr", "false"); e != nil {
		t.Fatal(e)
	}
	if e = s.applyDefaults(cmd); e != nil {
		t.Fatal(e)
	}
	if s.ShowPRs {
		t.Fatal("explicit false did not override inherited true")
	}
}
func TestFutureCommandDefaultsAreGenericAndHelpDoesNotLoadBrokenConfig(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	writeDefaults(t, filepath.Join(home, "config.toml"), "[commands.future]\nfeature=true\ndeadline='1s'\n")
	var out, logs bytes.Buffer
	s := New(core.New(root, home, &logs), &out, &logs)
	enabled := false
	var deadline time.Duration
	cmd := s.command("future", "Future feature", "A dynamically registered future command.", cobra.NoArgs, func(*cobra.Command, []string) (any, error) { return map[string]bool{"enabled": enabled}, nil })
	cmd.Flags().BoolVar(&enabled, "feature", false, "Opt into feature")
	cmd.Flags().DurationVar(&deadline, "deadline", 0, "Future duration flag")
	s.Root.AddCommand(cmd)
	if code := s.Execute(context.Background(), []string{"FUTURE", "-J"}); code != 0 || !enabled || deadline != time.Second {
		t.Fatalf("future defaults failed %s %s", out.String(), logs.String())
	}
	writeDefaults(t, filepath.Join(home, "config.toml"), "[commands.unknown]\nfeature=true\n")
	result, code, _ := invoke(t, core.New(root, home, nil), "list", "--help", "-j")
	if code != 0 || !result.OK {
		t.Fatal("help was blocked by configuration")
	}
}
func TestInvalidSelectedDefaultIsStructuredError(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	writeDefaults(t, filepath.Join(home, "config.toml"), "[commands.list]\nnot-a-flag=true\n")
	result, code, _ := invoke(t, core.New(root, home, nil), "list", "-j")
	if code == 0 || result.Error == nil || result.Error.Code != "invalid_config" {
		t.Fatalf("invalid config not reported %+v", result)
	}
}
