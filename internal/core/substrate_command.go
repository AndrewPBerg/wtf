package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const commandOutputLimit = 1 << 20

// CommandOptions configures bounded execution and the child environment.
type CommandOptions struct {
	Env          map[string]string
	Unset        []string
	NoInheritEnv bool
	Timeout      time.Duration
	Stdin        string
	Capture      bool
}

// CommandResult reports execution status and bounded captured output.
type CommandResult struct {
	Started         bool   `json:"started"`
	OK              bool   `json:"ok"`
	Code            int    `json:"code"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	StdoutTruncated bool   `json:"stdout_truncated"`
	StderrTruncated bool   `json:"stderr_truncated"`
	TimedOut        bool   `json:"timed_out"`
	Cancelled       bool   `json:"cancelled"`
}

type boundedOutput struct {
	data      []byte
	truncated bool
}

func (b *boundedOutput) Write(data []byte) (int, error) {
	n := len(data)
	remaining := commandOutputLimit - len(b.data)
	if len(data) > remaining {
		data = data[:remaining]
		b.truncated = true
	}
	b.data = append(b.data, data...)
	return n, nil
}

func validEnv(name, value string) error {
	if name == "" || strings.ContainsAny(name, "=\x00") || strings.ContainsRune(value, 0) {
		return Fail("invalid_environment", "Environment names must be nonempty and contain no equals sign or NUL; values contain no NUL")
	}
	return nil
}

func childEnvironment(set map[string]string, unset []string, inherit bool) ([]string, error) {
	values := map[string]string{}
	if inherit {
		for _, item := range os.Environ() {
			key, value, ok := strings.Cut(item, "=")
			if ok {
				values[key] = value
			}
		}
	}
	for _, key := range unset {
		if err := validEnv(key, ""); err != nil {
			return nil, err
		}
		delete(values, key)
	}
	for key, value := range set {
		if err := validEnv(key, value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+values[key])
	}
	return result, nil
}

// RunCommand executes argv directly. Captured contents never enter host records.
func (a *App) RunCommand(ctx context.Context, dir string, argv []string, options CommandOptions) (CommandResult, error) {
	result := CommandResult{Code: -1}
	if len(argv) == 0 || argv[0] == "" {
		return result, Fail("invalid_command", "Command argv must not be empty")
	}
	for _, arg := range argv {
		if strings.ContainsRune(arg, 0) {
			return result, Fail("invalid_command", "Command argv contains NUL")
		}
	}
	if options.Timeout == 0 {
		options.Timeout = 120 * time.Second
	}
	if options.Timeout < 0 || options.Timeout > time.Hour {
		return result, Fail("invalid_timeout", "Timeout must be greater than zero and at most 3600 seconds")
	}
	if len(options.Stdin) > fileByteLimit {
		return result, Fail("command_limit", "Command stdin exceeds 8 MiB")
	}
	env, err := childEnvironment(options.Env, options.Unset, !options.NoInheritEnv)
	if err != nil {
		return result, err
	}
	bounded, cancel := context.WithTimeout(ctx, options.Timeout)
	defer cancel()
	cmd := exec.CommandContext(bounded, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = strings.NewReader(options.Stdin)
	configureCommand(cmd)
	var stdout, stderr boundedOutput
	if options.Capture {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	} else {
		cmd.Stdout = a.Out
		cmd.Stderr = a.Out
	}
	err = cmd.Run()
	finishCommand(cmd)
	result.Stdout = string(stdout.data)
	result.Stderr = string(stderr.data)
	result.StdoutTruncated = stdout.truncated
	result.StderrTruncated = stderr.truncated
	if cmd.ProcessState != nil {
		result.Started = true
		result.Code = cmd.ProcessState.ExitCode()
	}
	result.TimedOut = ctx.Err() == nil && errors.Is(bounded.Err(), context.DeadlineExceeded)
	result.Cancelled = ctx.Err() != nil
	result.OK = err == nil && bounded.Err() == nil
	if result.Cancelled {
		return result, Fail("command_cancelled", ctx.Err().Error())
	}
	if result.TimedOut {
		return result, Fail("command_timeout", "Command exceeded its timeout")
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return result, Fail("command_failed", fmt.Sprintf("%s exited with code %d", filepath.Base(argv[0]), result.Code))
		}
		return result, Fail("command_failed", "Cannot execute command: "+err.Error())
	}
	return result, nil
}

// Keep interface verification close to the bounded writer.
var _ io.Writer = (*boundedOutput)(nil)
