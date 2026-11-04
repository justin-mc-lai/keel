//go:build windows

package tool

import "os/exec"

// setupProc is a no-op on Windows: process groups need job objects, which this
// base does not configure. WaitDelay still bounds the call.
func setupProc(cmd *exec.Cmd) {}
