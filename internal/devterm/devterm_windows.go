//go:build windows

package devterm

import (
	"os/exec"
	"strings"
	"syscall"
)

// createNewConsole — CREATE_NEW_CONSOLE: PowerShell открывается в
// собственном окне консоли. Прямой запуск вместо «cmd /c start …
// cmd /k powershell "…"»: вложенные кавычки cmd.exe ломали команду
// (окно мелькало и мгновенно закрывалось), а чужое окно cmd не нужно.
const createNewConsole = 0x00000010

// Terminal открывает новое окно PowerShell с командой. tail на Windows
// нет — файлы смотрим через Get-Content -Wait; окно остаётся открытым
// (-NoExit), пользователь закрывает сам.
func Terminal(command string, keepOpen bool) error {
	psCmd := strings.ReplaceAll(command, "'", "''")
	if keepOpen {
		psCmd += "; ''; Read-Host 'Press ENTER to close' | Out-Null"
	}
	cmd := exec.Command("powershell", "-NoProfile", "-NoExit",
		"-ExecutionPolicy", "Bypass", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
	return cmd.Start()
}

// ViewFile открывает файл в ассоциированном приложении.
func ViewFile(path string) error {
	return exec.Command("cmd", "/c", "start", "", path).Start()
}
