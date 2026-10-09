package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/core"
	"github.com/spf13/cobra"
)

func TestMixedCaseCommandsAliasesAndFlags(t *testing.T) {
	a := core.New(t.TempDir(), t.TempDir(), nil)
	for _, args := range [][]string{
		{"VeRsIoN", "--JsOn"},
		{"VERSION", "--JSON"},
		{"version", "-J"},
		{"version", "--JsOn=tRuE"},
		{"VERSION", "-J=tRuE"},
		{"VERSION", "-JH"},
		{"HoOk", "LiSt", "--JsOn", "--HeLp"},
		{"LS", "--JSON", "--HELP"},
		{"LIST", "--HOOK", "-J", "--HELP"},
		{"LS", "-KJ", "--HELP"},
		{"Sw", "--JsOn", "--hElP"},
		{"RM", "--JsOn", "--HELP"},
	} {
		result, code, _ := invoke(t, a, args...)
		if code != 0 || !result.OK {
			t.Fatalf("%v: %+v", args, result)
		}
	}
	result, code, _ := invoke(t, a, "VeRsIoN", "--UnknownFlag", "--JsOn")
	if code == 0 || result.Error == nil || result.Error.Code != "invalid_arguments" {
		t.Fatalf("mixed-case JSON must survive argument errors: %+v", result)
	}
}

func TestFutureCommandsInheritMiddlewareWithoutCustomCaseLogic(t *testing.T) {
	a := core.New(t.TempDir(), t.TempDir(), nil)
	var out, logs bytes.Buffer
	s := New(a, &out, &logs)
	group := s.group("future", "Future extension")
	group.Aliases = []string{"future-alias"}
	var path string
	var enabled bool
	leaf := s.command("leaf [ARGS...]", "Future command", "A command registered after middleware setup.", cobra.ArbitraryArgs,
		func(c *cobra.Command, args []string) (any, error) {
			return map[string]any{"path": path, "enabled": enabled, "argv": args}, nil
		})
	leaf.Aliases = []string{"leaf-alias"}
	leaf.Flags().StringVarP(&path, "ConfigPath", "C", "", "An opaque path")
	leaf.Flags().BoolVar(&enabled, "Enabled", false, "A future boolean")
	group.AddCommand(leaf)
	s.Root.AddCommand(group)
	code := s.Execute(context.Background(), []string{
		"FuTuRe-AlIaS", "LEAF-ALIAS", "--JsOn", "--cOnFiGpAtH=MiXeD/FILE.toml", "--eNaBlEd=tRuE",
		"--", "--JSON", "--From=KeepCase", "MiXeD",
	})
	if code != 0 {
		t.Fatalf("future command failed: %s %s", out.String(), logs.String())
	}
	var result struct {
		Command string `json:"command"`
		Data    struct {
			Path    string   `json:"path"`
			Enabled bool     `json:"enabled"`
			Argv    []string `json:"argv"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Command != "wtf future leaf" || result.Data.Path != "MiXeD/FILE.toml" || !result.Data.Enabled {
		t.Fatalf("future syntax or values changed: %+v", result)
	}
	if !reflect.DeepEqual(result.Data.Argv, []string{"--JSON", "--From=KeepCase", "MiXeD"}) {
		t.Fatalf("forwarded argv changed: %v", result.Data.Argv)
	}
	// Even an uppercase shorthand declared by a future command works in either case.
	input := prepareInput(s.Root, []string{"FUTURE", "LEAF", "-cMiXeD/FILE.toml", "-J"})
	if !reflect.DeepEqual(input.Args, []string{"future", "leaf", "-CMiXeD/FILE.toml", "-j"}) || !input.JSON {
		t.Fatalf("future shorthand not normalized: %+v", input)
	}
}

func TestMiddlewarePreservesFlagValuesAndChildArguments(t *testing.T) {
	a := core.New(t.TempDir(), t.TempDir(), nil)
	root := New(a, &bytes.Buffer{}, &bytes.Buffer{}).Root
	cases := []struct {
		args, want []string
		json       bool
	}{
		{[]string{"NeW", "MiXeD", "--FrOm", "Main@Origin", "--JsOn"}, []string{"new", "MiXeD", "--from", "Main@Origin", "--json"}, true},
		{[]string{"NeW", "MiXeD", "-FMain@Origin", "-J"}, []string{"new", "MiXeD", "-fMain@Origin", "-j"}, true},
		{[]string{"NeW", "MiXeD", "-F", "Main@Origin", "-J"}, []string{"new", "MiXeD", "-f", "Main@Origin", "-j"}, true},
		{[]string{"NeW", "MiXeD", "--FrOm=Main@Origin", "--JSON=FaLsE"}, []string{"new", "MiXeD", "--from=Main@Origin", "--json=false"}, false},
		{[]string{"NeW", "MiXeD", "--FrOm", "--JsOn"}, []string{"new", "MiXeD", "--from", "--JsOn"}, false},
		{[]string{"RuN", "MiXeDTask", "--", "--JsOn", "--FrOm", "VALUE"}, []string{"run", "MiXeDTask", "--", "--JsOn", "--FrOm", "VALUE"}, false},
	}
	for _, tc := range cases {
		input := prepareInput(root, tc.args)
		if !reflect.DeepEqual(input.Args, tc.want) || input.JSON != tc.json {
			t.Fatalf("%v normalized incorrectly: %+v want %v JSON=%v", tc.args, input, tc.want, tc.json)
		}
	}
}

func TestHookArgumentsMustFollowSeparatorAndKeepCase(t *testing.T) {
	a := core.New(t.TempDir(), t.TempDir(), nil)
	root := New(a, &bytes.Buffer{}, &bytes.Buffer{}).Root
	input := prepareInput(root, []string{"NeW", "NameCase", "-J", "--", "--Compose", "ValueCase"})
	if !reflect.DeepEqual(input.Args, []string{"new", "NameCase", "-j", "--", "--Compose", "ValueCase"}) {
		t.Fatalf("hook arguments changed %+v", input)
	}
	for _, args := range [][]string{{"new", "one", "extra", "-j"}, {"hook", "run", "one", "extra", "-j"}} {
		result, code, _ := invoke(t, a, args...)
		if code == 0 || result.Error == nil || result.Error.Code != "invalid_arguments" {
			t.Fatalf("accepted unseparated arguments %v: %+v", args, result)
		}
	}
}
