//go:build windows

package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// CREATE_NO_WINDOW: сам cmd.exe не должен мигать консолью, окно создаёт start.
const createNoWindow = 0x08000000

// HasGUI: на Windows графическая сессия есть всегда, если запущен explorer.
func HasGUI() bool { return true }

// LaunchWindow открывает TUI в новом окне консоли через cmd start.
// Размер окна задаётся настройками консоли, а не аргументами процесса.
func LaunchWindow(_, _ int, _ string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("не удалось определить путь к бинарнику: %w", err)
	}
	if info, err := os.Stat(exe); err != nil {
		return fmt.Errorf("не найден %s: %w", exe, err)
	} else if info.IsDir() {
		return fmt.Errorf("%s — это каталог, а не исполняемый файл", exe)
	}

	dir := filepath.Dir(exe)
	comspec := os.Getenv("ComSpec")
	if comspec == "" {
		comspec = "cmd.exe"
	}

	cmd := exec.Command(comspec)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       buildStartLine(comspec, dir, exe),
		CreationFlags: createNoWindow,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("не удалось открыть окно терминала: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}
