package setup

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetect_Override(t *testing.T) {
	tests := []struct {
		override string
		want     Shell
		wantErr  bool
	}{
		{"bash", Bash, false},
		{"zsh", Zsh, false},
		{"fish", Fish, false},
		{"BASH", Bash, false},
		{"ZSH", Zsh, false},
		{"ksh", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.override, func(t *testing.T) {
			d := &ShellDetector{GetEnv: func(string) string { return "" }}
			got, err := d.Detect(tt.override)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetect_EnvShell(t *testing.T) {
	tests := []struct {
		env  string
		want Shell
	}{
		{"/bin/bash", Bash},
		{"/usr/bin/zsh", Zsh},
		{"/usr/local/bin/fish", Fish},
		{"/bin/zsh", Zsh},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			d := &ShellDetector{
				GetEnv: func(key string) string {
					if key == "SHELL" {
						return tt.env
					}
					return ""
				},
			}
			got, err := d.Detect("")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetect_ParentProcess(t *testing.T) {
	d := &ShellDetector{
		GetEnv: func(string) string { return "" },
		ReadParentComm: func(_ int) (string, error) {
			return "zsh", nil
		},
	}
	got, err := d.Detect("")
	require.NoError(t, err)
	assert.Equal(t, Zsh, got)
}

func TestDetect_ParentProcessFallthrough(t *testing.T) {
	d := &ShellDetector{
		GetEnv: func(string) string { return "" },
		ReadParentComm: func(_ int) (string, error) {
			return "", fmt.Errorf("no such file")
		},
	}
	_, err := d.Detect("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not detect shell")
}

func TestDetect_ParentProcessUnsupportedShell(t *testing.T) {
	d := &ShellDetector{
		GetEnv: func(string) string { return "" },
		ReadParentComm: func(_ int) (string, error) {
			return "tcsh", nil
		},
	}
	_, err := d.Detect("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "could not detect shell")
}

func TestDetect_LoginShellPrefix(t *testing.T) {
	d := &ShellDetector{
		GetEnv: func(key string) string {
			if key == "SHELL" {
				return "/bin/-bash"
			}
			return ""
		},
	}
	got, err := d.Detect("")
	require.NoError(t, err)
	assert.Equal(t, Bash, got)
}

func TestRender_BashZsh(t *testing.T) {
	funcs := DefaultFuncs()

	for _, shell := range []Shell{Bash, Zsh} {
		t.Run(string(shell), func(t *testing.T) {
			out := Render(shell, funcs)
			assert.Contains(t, out, `wtf()`)
			assert.Contains(t, out, `command wtf sw "$@"`)
		})
	}
}

func TestRender_Fish(t *testing.T) {
	out := Render(Fish, DefaultFuncs())
	assert.Contains(t, out, "function wtf")
	assert.Contains(t, out, "command wtf sw $argv[2..]")
	assert.Contains(t, out, "end")
}

func TestRender_MultipleFuncs(t *testing.T) {
	funcs := []ShellFunc{
		{Name: "a", Bash: "a() { echo a; }", Fish: "function a; echo a; end"},
		{Name: "b", Bash: "b() { echo b; }", Fish: "function b; echo b; end"},
	}
	out := Render(Bash, funcs)
	assert.Contains(t, out, "a() { echo a; }")
	assert.Contains(t, out, "b() { echo b; }")
}

func TestDefaultFuncs(t *testing.T) {
	funcs := DefaultFuncs()
	require.Len(t, funcs, 1)
	assert.Equal(t, "wtf", funcs[0].Name)
	assert.NotEmpty(t, funcs[0].Bash)
	assert.NotEmpty(t, funcs[0].Fish)
}

func TestParseShell_Unsupported(t *testing.T) {
	_, err := parseShell("powershell")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported shell")
}

func TestShellBasename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/usr/bin/bash", "bash"},
		{"bash", "bash"},
		{"/bin/zsh", "zsh"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, shellBasename(tt.input))
	}
}

func TestNewShellDetector(t *testing.T) {
	d := NewShellDetector()
	require.NotNil(t, d)
	require.NotNil(t, d.GetEnv)
	require.NotNil(t, d.ReadParentComm)
}

func TestDefaultReadParentComm(t *testing.T) {
	// Reading own process comm should work on Linux
	comm, err := defaultReadParentComm(os.Getpid())
	if err != nil {
		t.Skip("not on Linux or /proc not available")
	}
	assert.NotEmpty(t, comm)
}

func TestDefaultReadParentComm_InvalidPID(t *testing.T) {
	_, err := defaultReadParentComm(999999999)
	assert.Error(t, err)
}
