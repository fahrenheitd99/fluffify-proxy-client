//go:build windows

package devterm

import (
	"fmt"
	"os/exec"
	"strings"
)

// Terminal открывает новое консольное окно с командой. tail на Windows
// нет — файлы смотрим через PowerShell Get-Content -Wait; окно остаётся
// открытым (-NoExit), пользователь закрывает сам.
func Terminal(command string, keepOpen bool) error {
	psCmd := strings.ReplaceAll(command, "'", "''")
	if keepOpen {
		psCmd += "; ''; Read-Host 'Press ENTER to close' | Out-Null"
	}
	inner := fmt.Sprintf(`powershell -NoExit -ExecutionPolicy Bypass -Command "%s"`, psCmd)
	return exec.Command("cmd", "/c", "start", "Fluffify", "cmd", "/k", inner).Start()
}

// ViewFile открывает файл в ассоциированном приложении.
func ViewFile(path string) error {
	return exec.Command("cmd", "/c", "start", "", path).Start()
}
