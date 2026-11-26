//go:build windows

package about

import (
	"os"
	"os/exec"
	"path/filepath"

	"xray-tui/internal/osutil"
)

// OpenLicenseFile извлекает встроенные лицензии в скрытую папку пользователя
// (%LocalAppData%\Fluffify\licenses) — перезаписывая файл, чтобы текст всегда
// был актуальным — и открывает его системным приложением. Процесс запускается
// отвязанным: TUI не ждёт закрытия Блокнота.
func OpenLicenseFile() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		// LOCALAPPDATA не задан — запасной вариант.
		cache, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		base = cache
	}
	dir := filepath.Join(base, "Fluffify", "licenses")

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "licenses.txt")
	if err := os.WriteFile(path, []byte(Combined()), 0o644); err != nil {
		return "", err
	}

	// cmd /c start закрывает cmd сразу, Блокнот открывается отдельно.
	cmd := exec.Command("cmd.exe", "/c", "start", "", path)
	osutil.Detach(cmd)
	if err := cmd.Start(); err != nil {
		return path, err
	}
	return path, nil
}
