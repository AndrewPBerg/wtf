package cli

import (
	"bytes"
	"testing"

	"github.com/AndrewPBerg/wtf/internal/setup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitCommand_Bash(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := initCmd
	cmd.SetOut(buf)

	detector := &setup.ShellDetector{
		GetEnv: func(key string) string {
			if key == "SHELL" {
				return "/bin/bash"
			}
			return ""
		},
	}

	err := runInit(cmd, "bash", detector)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `wtf()`)
	assert.Contains(t, buf.String(), `command wtf "$_c" "$@"`)
}

func TestInitCommand_Zsh(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := initCmd
	cmd.SetOut(buf)

	detector := &setup.ShellDetector{
		GetEnv: func(string) string { return "" },
	}

	err := runInit(cmd, "zsh", detector)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `wtf()`)
}

func TestInitCommand_Fish(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := initCmd
	cmd.SetOut(buf)

	detector := &setup.ShellDetector{
		GetEnv: func(string) string { return "" },
	}

	err := runInit(cmd, "fish", detector)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "function wtf")
	assert.Contains(t, buf.String(), "end")
}

func TestInitCommand_InvalidShell(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := initCmd
	cmd.SetOut(buf)

	detector := &setup.ShellDetector{
		GetEnv: func(string) string { return "" },
	}

	err := runInit(cmd, "powershell", detector)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported shell")
}

func TestInitCommand_DetectFromEnv(t *testing.T) {
	buf := new(bytes.Buffer)
	cmd := initCmd
	cmd.SetOut(buf)

	detector := &setup.ShellDetector{
		GetEnv: func(key string) string {
			if key == "SHELL" {
				return "/usr/bin/zsh"
			}
			return ""
		},
	}

	err := runInit(cmd, "", detector)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `wtf()`)
}
