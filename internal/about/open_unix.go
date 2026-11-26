//go:build !windows

package about

import (
	"os"
	"os/exec"
	"path/filepath"
)

// OpenLicenseFile извлекает встроенные лицензии в скрытую папку пользователя
// (~/.cache/fluffify/licenses) — перезаписывая файл, чтобы текст всегда был
// актуальным — и открывает его через xdg-open. Процесс запускается отвязанным:
// TUI не ждёт закрытия просмотрщика.
func OpenLicenseFile() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "fluffify", "licenses")

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "licenses.txt")
	if err := os.WriteFile(path, []byte(Combined()), 0o644); err != nil {
		return "", err
	}

	xdg, err := exec.LookPath("xdg-open")
	if err != nil {
		return path, err // файл записан, но просмотрщика в системе нет
	}
	cmd := exec.Command(xdg, path)
	if err := cmd.Start(); err != nil {
		return path, err
	}
	return path, nil
}
