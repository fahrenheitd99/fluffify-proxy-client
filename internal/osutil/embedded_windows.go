//go:build windows

package osutil

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"os"
	"path/filepath"
)

//go:embed assets/xray.exe.gz
var xrayExeGZ []byte

//go:embed assets/wintun.dll
var wintunDLL []byte

// EnsureCoreFiles извлекает встроенные ядро (xray.exe) и драйвер TUN
// (wintun.dll) в %LocalAppData%\Fluffify — там же они и живут: exe можно
// носить куда угодно отдельно от папки. Возвращает каталог с файлами.
//
// Файлы перезаписываются, только если отсутствуют или изменился размер:
// повторное извлечение 35 МБ на каждый запуск не нужно.
func EnsureCoreFiles() (string, error) {
	cache, err := os.UserCacheDir() // Windows: %LocalAppData%
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "Fluffify", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	// wintun.dll — драйвер TUN.
	dllPath := filepath.Join(dir, "wintun.dll")
	if info, err := os.Stat(dllPath); err != nil || int64(len(wintunDLL)) != info.Size() {
		if err := os.WriteFile(dllPath, wintunDLL, 0o644); err != nil {
			return "", err
		}
	}

	// xray.exe — ядро (распаковываем из встроенного gzip).
	exeBytes, err := gunzipBytes(xrayExeGZ)
	if err != nil {
		return "", err
	}
	exePath := filepath.Join(dir, "xray.exe")
	if info, err := os.Stat(exePath); err != nil || int64(len(exeBytes)) != info.Size() {
		if err := os.WriteFile(exePath, exeBytes, 0o755); err != nil {
			return "", err
		}
	}

	return dir, nil
}

func gunzipBytes(data []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

// CoreBinPath — путь к встроенному ядру xray.exe. Пустая строка не
// возвращается: если извлечение не удалось, всё равно отдаём путь в
// %LocalAppData% (клиент покажет ошибку запуска ядра).
func CoreBinPath() string {
	cache := os.Getenv("LOCALAPPDATA")
	if cache == "" {
		return "xray.exe"
	}
	return filepath.Join(cache, "Fluffify", "bin", "xray.exe")
}
