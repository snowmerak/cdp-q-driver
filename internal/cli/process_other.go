//go:build !windows && !unix

package cli

import "os/exec"

func backgroundProcess(cmd *exec.Cmd) {}
