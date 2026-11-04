//go:build !windows

package jobs

import (
	"os/exec"
	"syscall"
)

// setupProc puts the command in its own group so a kill reaches children.
func setupProc(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
