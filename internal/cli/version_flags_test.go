package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
)

func TestVersionFlagsMatchVersionCommandWithoutProjectSetup(t *testing.T) {
	a := core.New(t.TempDir(), t.TempDir(), nil)
	if err := os.Mkdir(filepath.Join(a.Cwd, ".wtf"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.Cwd, ".wtf", "config.toml"), []byte("invalid TOML ["), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"version", "-j"}, {"--version", "-j"}, {"-v", "--json"}, {"--VeRsIoN", "-J"}, {"-VJ"}} {
		result, code, _ := invoke(t, a, args...)
		if code != 0 || !result.OK || result.Command != "wtf version" {
			t.Fatalf("version failed for %v: %+v", args, result)
		}
		data, ok := result.Data.(map[string]any)
		if !ok || data["version"] != Version {
			t.Fatalf("wrong version for %v: %+v", args, result.Data)
		}
	}
	for _, flag := range []string{"--version", "-v"} {
		var out, logs bytes.Buffer
		s := New(a, &out, &logs)
		if code := s.Execute(t.Context(), []string{flag}); code != 0 || strings.TrimSpace(out.String()) != "WTF "+Version || logs.Len() != 0 {
			t.Fatalf("human version failed for %s: %s %s", flag, &out, &logs)
		}
	}
}
