//go:build windows

package cli

import (
	"os/exec"
	"syscall"
)

func backgroundProcess(cmd *exec.Cmd) {
	// DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP. Child stdin is immediate EOF.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008 | 0x00000200}
}
