//go:build !windows

package tool

import (
	"os/exec"
	"syscall"
)

// setupProc puts the command in its own process group and kills the whole
// group on cancel. Without this, `sh -c "cmd &"` leaves children alive after
// the deadline: WaitDelay bounds our wait, not the process tree.
func setupProc(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
