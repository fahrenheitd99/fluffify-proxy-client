//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"xray-tui/internal/osutil"
	"xray-tui/internal/tui"
)

const tunCaps = "cap_net_admin,cap_net_raw+ep"

// runTUNSetup выдаёт права на TUN бинарнику ядра xray: systemd-юнит пользователя
// не может дать права демону (user-менеджер не владеет CAP_NET_ADMIN), поэтому
// права должны лежать на самом xray — setcap через pkexec (в GUI) или sudo.
func runTUNSetup() error {
	xrayPath := osutil.ResolveBinary("xray")
	if !filepath.IsAbs(xrayPath) {
		return errors.New("ядро xray не найдено ни рядом с приложением, ни в PATH")
	}
	if resolved, err := filepath.EvalSymlinks(xrayPath); err == nil {
		if _, err := os.Stat(resolved); err == nil {
			xrayPath = resolved
		}
	}

	if xrayHasFileCaps(xrayPath) {
		fmt.Printf("Права на TUN уже есть: %s\n", xrayPath)
		return nil
	}

	fmt.Printf("Выдаю права на TUN (%s) для %s...\n", tunCaps, xrayPath)
	var cmd *exec.Cmd
	if _, err := exec.LookPath("pkexec"); err == nil && tui.HasGUI() {
		cmd = exec.Command("pkexec", "setcap", tunCaps, xrayPath)
	} else {
		fmt.Println("pkexec недоступен — запускаю sudo (введи пароль в терминале):")
		cmd = exec.Command("sudo", "setcap", tunCaps, xrayPath)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("не удалось выдать права (setcap %s): %w", xrayPath, err)
	}

	if !xrayHasFileCaps(xrayPath) {
		return fmt.Errorf("setcap отработал, но права не видны на %s — перезапусти демон и повтори", xrayPath)
	}
	fmt.Println("Готово. Перезапусти демон (выйди из приложения и запусти заново) — TUN заработает.")
	return nil
}

// xrayHasFileCaps проверяет файловые права ядра через getcap.
func xrayHasFileCaps(path string) bool {
	if _, err := exec.LookPath("getcap"); err != nil {
		return false
	}
	out, err := exec.Command("getcap", path).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "cap_net_admin")
}
