//go:build windows

package osutil

import (
	"os/exec"
	"syscall"
)

// createNoWindow — CREATE_NO_WINDOW: дочерний консольный процесс
// (xray.exe, cmd) не вспыхивает окном консоли, когда родитель (демон,
// запущенный detached) не имеет собственной консоли.
const createNoWindow = 0x08000000

// HideConsole подавляет окно консоли у текущего процесса (демон/tray).
// HideChildConsole — то же для запускаемых дочерних процессов. Вызывается
// для всех фоновых exec'ов: xray run, xray version, cmd-утилиты.
func HideChildConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
}
