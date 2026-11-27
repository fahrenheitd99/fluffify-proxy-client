//go:build !windows

package osutil

import (
	"os"
	"os/exec"
	"path/filepath"
)

// ResolveBinary ищет ядро сначала в PATH — в Linux xray ставится системным
// пакетом в /usr/bin, и ссылаться нужно именно на него (setcap и права
// привязаны к этому файлу). Бинарник рядом с приложением — запасной вариант
// для portable-запуска без системного пакета.
func ResolveBinary(base string) string {
	name := ExeName(base)
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return name
}
