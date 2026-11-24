package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func HasGUI() bool {
	return os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("DISPLAY") != ""
}

func LaunchWindow(cols, lines int, preferredTerm string) error {
	if !HasGUI() {
		return errors.New("графическая сессия не обнаружена")
	}
	if cols <= 0 {
		cols = 48
	}
	if lines <= 0 {
		lines = 34
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("не удалось определить путь к бинарнику: %w", err)
	}

	candidates := []string{"alacritty", "kitty", "foot", "konsole"}
	if preferredTerm != "" {
		candidates = append([]string{preferredTerm}, candidates...)
	}

	var termName, termPath string
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			termName = c
			termPath = p
			break
		}
	}
	if termPath == "" {
		return errors.New("не найден поддерживаемый терминал (alacritty, kitty, foot, konsole)")
	}

	var args []string
	switch termName {
	case "alacritty":
		args = []string{
			"--class", "xray-tui",
			"--title", "Fluffify",
			"-o", fmt.Sprintf("window.dimensions.columns=%d", cols),
			"-o", fmt.Sprintf("window.dimensions.lines=%d", lines),
			"-e", exe, "tui",
		}
	case "kitty":
		args = []string{
			"--class", "xray-tui",
			"--title", "Fluffify",
			"-o", "remember_window_size=no",
			"-o", fmt.Sprintf("initial_window_width=%dc", cols),
			"-o", fmt.Sprintf("initial_window_height=%dc", lines),
			exe, "tui",
		}
	case "foot":
		args = []string{
			"--app-id=xray-tui",
			"--title=Fluffify",
			fmt.Sprintf("--window-size-chars=%dx%d", cols, lines),
			exe, "tui",
		}
	case "konsole":
		args = []string{
			"--hide-menubar",
			"--hide-tabbar",
			"-p", fmt.Sprintf("TerminalColumns=%d", cols),
			"-p", fmt.Sprintf("TerminalRows=%d", lines),
			"-e", exe, "tui",
		}
	default:
		args = []string{"-e", exe, "tui"}
	}

	cmd := exec.Command(termPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("не удалось открыть окно %s: %w", termName, err)
	}
	_ = cmd.Process.Release()
	return nil
}