//go:build !windows

package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const unitName = "xray-tui.service"

// Enabled сообщает, включён ли systemd-юнит пользователя.
func Enabled() bool {
	return exec.Command("systemctl", "--user", "is-enabled", "--quiet", unitName).Run() == nil
}

// Set включает или выключает автозапуск демона при входе в систему.
func Set(enable bool) error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("systemctl не найден")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	unitDir := filepath.Join(home, ".config", "systemd", "user")
	unitPath := filepath.Join(unitDir, unitName)

	if enable {
		if _, err := os.Stat(unitPath); os.IsNotExist(err) {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			if resolved, err := filepath.EvalSymlinks(exe); err == nil {
				exe = resolved
			}
			if err := os.MkdirAll(unitDir, 0o755); err != nil {
				return err
			}
			unit := fmt.Sprintf(`[Unit]
Description=Xray TUI Background Daemon
After=network-online.target

[Service]
Type=simple
ExecStart=%s daemon
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
`, exe)
			if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
				return err
			}
		}
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		if out, err := exec.Command("systemctl", "--user", "enable", unitName).CombinedOutput(); err != nil {
			return fmt.Errorf("%s", strings.TrimSpace(string(out)))
		}
		return nil
	}

	if out, err := exec.Command("systemctl", "--user", "disable", unitName).CombinedOutput(); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(string(out)))
	}
	return nil
}

// Start поднимает демон через systemd. false — юнит не установлен или systemd нет.
func Start() bool {
	return exec.Command("systemctl", "--user", "start", unitName).Run() == nil
}

// Stop останавливает юнит, чтобы systemd не перезапустил демон после выхода из трея.
func Stop() {
	_ = exec.Command("systemctl", "--user", "stop", unitName).Run()
}
