package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectShell(t *testing.T) {
	tests := []struct {
		name    string
		shell   string
		want    string
		wantErr string
	}{
		{name: "bash", shell: "/bin/bash", want: "bash"},
		{name: "zsh", shell: "/usr/bin/zsh", want: "zsh"},
		{name: "fish", shell: "/usr/local/bin/fish", want: "fish"},
		{name: "pwsh", shell: "/usr/bin/pwsh", want: "powershell"},
		{name: "empty", shell: "", wantErr: "$SHELL is not set"},
		{name: "unsupported", shell: "/bin/csh", wantErr: "unsupported shell"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SHELL", tt.shell)
			got, err := detectShell()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func resetCompletionFlags() {
	_ = completionCmd.Flags().Set("shell", "")
}

func TestCompletionCommand_ExplicitShell(t *testing.T) {
	tests := []struct {
		shell    string
		contains string
	}{
		{"bash", "bash completion"},
		{"zsh", "zsh"},
		{"fish", "fish"},
		{"powershell", "powershell"},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			t.Cleanup(resetCompletionFlags)
			buf := new(bytes.Buffer)
			rootCmd.SetOut(buf)
			rootCmd.SetArgs([]string{"completion", "--shell", tt.shell})

			err := rootCmd.Execute()
			require.NoError(t, err)
			assert.Contains(t, buf.String(), tt.contains)
		})
	}
}

func TestCompletionCommand_AutoDetect(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"completion"})

	err := rootCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "bash completion")
}

func TestCompletionCommand_AutoDetectZsh(t *testing.T) {
	t.Setenv("SHELL", "/usr/bin/zsh")
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetArgs([]string{"completion"})

	err := rootCmd.Execute()
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "zsh")
}

func TestCompletionCommand_NoShell(t *testing.T) {
	t.Setenv("SHELL", "")
	rootCmd.SetArgs([]string{"completion"})

	err := rootCmd.Execute()
	require.Error(t, err)
}

func TestCompletionCommand_UnsupportedShell(t *testing.T) {
	t.Setenv("SHELL", "/bin/csh")
	rootCmd.SetArgs([]string{"completion"})

	err := rootCmd.Execute()
	require.Error(t, err)
}

func TestCompletionCommand_InvalidShellFlag(t *testing.T) {
	t.Cleanup(resetCompletionFlags)
	rootCmd.SetArgs([]string{"completion", "--shell", "tcsh"})

	err := rootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported shell")
}
