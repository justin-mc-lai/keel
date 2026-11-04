//go:build windows

package jobs

import "os/exec"

// setupProc is a no-op on Windows; WaitDelay still bounds the call.
func setupProc(cmd *exec.Cmd) {}
