//go:build !linux

package core

import (
	"context"
	"os/exec"
)

func lock(context.Context, string) (func(), error) {
	return nil, Fail("unsupported_platform", "This local pilot currently supports Linux")
}
func configureCommand(cmd *exec.Cmd) {}
func finishCommand(cmd *exec.Cmd)    {}
