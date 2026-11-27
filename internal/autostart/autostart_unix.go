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
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}

		// Юнит мог остаться от другой сборки (например, из старого каталога
		// проекта) или от старой версии с неработающими AmbientCapabilities:
		// тогда systemd поднимал бы устаревшего демона или падал с
		// 218/CAPABILITIES. Перезаписываем в обоих случаях.
		current, readErr := os.ReadFile(unitPath)
		if readErr != nil || !unitMatchesExe(string(current), exe) ||
			strings.Contains(string(current), "AmbientCapabilities") {
			if err := os.MkdirAll(unitDir, 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(unitPath, []byte(unitContent(exe)), 0o644); err != nil {
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

// unitContent — текст user-юнита, запускающего демон из указанного бинарника.
// Без AmbientCapabilities: user-менеджер systemd не владеет CAP_NET_ADMIN,
// юнит с такими правами не запускается вовсе (218/CAPABILITIES). Права на
// TUN выдаёт tun-setup: setcap на бинарнике ядра xray.
func unitContent(exe string) string {
	return fmt.Sprintf(`[Unit]
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
}

// unitMatchesExe проверяет, что юнит запускает именно этот бинарник.
func unitMatchesExe(content, exe string) bool {
	return strings.Contains(content, "ExecStart="+exe+" ")
}

// Start поднимает демон через systemd. false — юнит не установлен или systemd нет.
// Перед стартом перечитываем юниты: пакет мог обновить /usr/lib/systemd/user
// (например, убрать неработающие AmbientCapabilities старой версии) — без
// daemon-reload systemd продолжал бы рестарт-луп старого описания.
func Start() bool {
	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	_ = exec.Command("systemctl", "--user", "reset-failed", unitName).Run()
	return exec.Command("systemctl", "--user", "start", unitName).Run() == nil
}

// Stop останавливает юнит, чтобы systemd не перезапустил демон после выхода из трея.
func Stop() {
	_ = exec.Command("systemctl", "--user", "stop", unitName).Run()
}
