//go:build !windows

package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

const (
	// unitName — user-юнит демона (без прав, прокси-режим; TUN через setcap).
	unitName = "xray-tui.service"

	// systemUnitPath — пакетный system-юнит fluffify-daemon@<user>: демон
	// работает от имени пользователя с минимальным набором кап
	// (CAP_NET_ADMIN, CAP_NET_RAW). TUN и перехват DNS работают без паролей
	// и без setcap; управление юнитом разрешено polkit-правилом пакета
	// (packaging/50-fluffify-daemon.rules).
	systemUnitPath = "/usr/lib/systemd/system/fluffify-daemon@.service"
)

// SystemDaemonUnitAvailable сообщает, установлен ли пакетный system-юнит
// демона. Если да — демон поднимается через него, а user-юнит гасится:
// иначе оба демона боролись бы за один IPC-сокет.
func SystemDaemonUnitAvailable() bool {
	if _, err := os.Stat(systemUnitPath); err != nil {
		return false
	}
	_, err := exec.LookPath("systemctl")
	return err == nil
}

// currentUserName — имя пользователя для экземпляра fluffify-daemon@<name>.
func currentUserName() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return os.Getenv("USER")
}

// systemUnitInstance — имя экземпляра шаблонного system-юнита.
func systemUnitInstance() string {
	return "fluffify-daemon@" + currentUserName() + ".service"
}

// Enabled сообщает, обеспечен ли автозапуск демона. С system-юнитом демон
// поднимается по требованию без пароля, так что автозапуск считается
// обеспеченным всегда.
func Enabled() bool {
	if SystemDaemonUnitAvailable() {
		return true
	}
	return exec.Command("systemctl", "--user", "is-enabled", "--quiet", unitName).Run() == nil
}

// Set включает или выключает автозапуск демона при входе в систему.
// С system-юнитом управление автозапуском не требуется (демон стартует по
// требованию из приложения без пароля) — обе операции no-op.
func Set(enable bool) error {
	if SystemDaemonUnitAvailable() {
		return nil
	}

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

// Start поднимает демон. С пакетным system-юнитом — через systemctl start
// (polkit-правило пакета снимает запрос пароля для wheel в активной сессии);
// /run/user/<uid> к этому моменту существует, поскольку вызов идёт из
// приложения после входа пользователя. Без пакета — через user-юнит.
// Перед стартом перечитываем юниты: пакет мог обновить юнит-файлы — без
// daemon-reload systemd продолжал бы рестарт-луп старого описания.
func Start() bool {
	if SystemDaemonUnitAvailable() {
		// Прошлая схема могла оставить включённый user-юнит: его демон
		// держал бы IPC-сокет и мешал system-демону. Гасим best-effort —
		// это юзерские права, пароль не нужен.
		_ = exec.Command("systemctl", "--user", "disable", "--now", unitName).Run()
		_ = exec.Command("systemctl", "reset-failed", systemUnitInstance()).Run()
		return exec.Command("systemctl", "start", systemUnitInstance()).Run() == nil
	}

	_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
	_ = exec.Command("systemctl", "--user", "reset-failed", unitName).Run()
	return exec.Command("systemctl", "--user", "start", unitName).Run() == nil
}

// Stop останавливает демона, чтобы systemd не перезапустил его после
// выхода из трея.
func Stop() {
	if SystemDaemonUnitAvailable() {
		_ = exec.Command("systemctl", "stop", systemUnitInstance()).Run()
		return
	}
	_ = exec.Command("systemctl", "--user", "stop", unitName).Run()
}
