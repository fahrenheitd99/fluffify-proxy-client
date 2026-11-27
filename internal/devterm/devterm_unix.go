//go:build !windows

package devterm

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Terminal запускает command в новом окне терминала. Для завершающихся
// команд окно держится открытым до нажатия Enter (keepOpen).
func Terminal(command string, keepOpen bool) error {
	if env := os.Getenv("TERMINAL"); env != "" {
		if err := spawnEmu(env, nil, command, keepOpen); err == nil {
			return nil
		}
	}
	var firstErr error
	for _, emu := range emulators {
		bin, err := exec.LookPath(emu.name)
		if err != nil {
			continue
		}
		if err = spawnEmu(bin, emu.args, command, keepOpen); err == nil {
			return nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return firstErr
	}
	return fmt.Errorf("не найден эмулятор терминала (kitty/alacritty/foot/gnome-terminal/konsole/xterm)")
}

// ViewFile открывает файл в системном просмотрщике/редакторе.
func ViewFile(path string) error {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/c", "start", "", path).Start()
	}
	xdg, _ := exec.LookPath("xdg-open")
	if xdg == "" {
		return Terminal("less "+shellQuote(path), true)
	}
	return exec.Command(xdg, path).Start()
}

type emulator struct {
	name string
	// args — как передать команду: %c подставляется как одна строка
	// (sh -c '...'), элементы вокруг — статичные аргументы эмулятора.
	args []string
}

var emulators = []emulator{
	{"kitty", []string{"sh", "-c", "%c"}},
	{"alacritty", []string{"-e", "sh", "-c", "%c"}},
	{"foot", []string{"sh", "-c", "%c"}},
	{"wezterm", []string{"start", "--", "sh", "-c", "%c"}},
	{"gnome-terminal", []string{"--", "sh", "-c", "%c"}},
	{"konsole", []string{"-e", "sh", "-c", "%c"}},
	{"tilix", []string{"-e", "sh", "-c", "%c"}},
	{"xfce4-terminal", []string{"-x", "sh", "-c", "%c"}},
	{"mate-terminal", []string{"-e", "sh", "-c", "%c"}},
	{"lxterminal", []string{"-e", "sh", "-c", "%c"}},
	{"xterm", []string{"-e", "sh", "-c", "%c"}},
	{"x-terminal-emulator", []string{"-e", "sh", "-c", "%c"}},
}

func spawnEmu(bin string, args []string, command string, keepOpen bool) error {
	cmdLine := command
	if keepOpen {
		// POSIX-совместимо: read line работает в dash/bash.
		cmdLine = fmt.Sprintf("%s; echo; echo '[Fluffify] press ENTER to close'; read _", command)
	}
	full := make([]string, 0, len(args)+1)
	for _, a := range args {
		if a == "%c" {
			full = append(full, cmdLine)
		} else {
			full = append(full, a)
		}
	}
	if len(args) == 0 {
		// Пользовательский $TERMINAL без описания формата: пробуем как есть.
		full = []string{"-e", "sh", "-c", cmdLine}
	}
	return exec.Command(bin, full...).Start()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
