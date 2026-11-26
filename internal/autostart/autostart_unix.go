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
		// проекта): тогда systemd поднимал бы устаревший демон. Перезаписываем,
		// если путь не совпадает с текущим бинарником.
		current, readErr := os.ReadFile(unitPath)
		if readErr != nil || !unitMatchesExe(string(current), exe) {
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
// AmbientCapabilities даёт демону сетевые права: запущенный им child-процесс
// xray наследует их и сам создаёт TUN-адаптер, не поднимаясь до root.
func unitContent(exe string) string {
	return fmt.Sprintf(`[Unit]
Description=Xray TUI Background Daemon
After=network-online.target

[Service]
Type=simple
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW
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
func Start() bool {
	return exec.Command("systemctl", "--user", "start", unitName).Run() == nil
}

// Stop останавливает юнит, чтобы systemd не перезапустил демон после выхода из трея.
func Stop() {
	_ = exec.Command("systemctl", "--user", "stop", unitName).Run()
}
