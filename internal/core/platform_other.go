//go:build !linux

package core

import (
	"context"
	"os"
	"os/exec"
)

func lock(context.Context, string) (func(), error) {
	return nil, Fail("unsupported_platform", "This local pilot currently supports Linux")
}
func configureCommand(cmd *exec.Cmd) {}
func finishCommand(cmd *exec.Cmd)    {}

func commitNewDirectory(*os.Root, string, string) error {
	return Fail("unsupported_platform", "Atomic directory copy currently supports Linux")
}

func rootReadFlags() int { return os.O_RDONLY }
