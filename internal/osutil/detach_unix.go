//go:build !windows

package osutil

import (
	"os/exec"
	"syscall"
)

// Detach запускает процесс в собственной сессии, чтобы он пережил закрытие родителя.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
