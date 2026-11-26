//go:build windows

package osutil

import (
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

// Detach отвязывает процесс от консоли родителя: без этого дочерний демон
// и трей завершаются вместе с окном, из которого их запустили.
func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | createNewProcessGroup,
		HideWindow:    true,
	}
}
