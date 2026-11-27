//go:build windows

package osutil

import (
	"os"
	"os/exec"
	"path/filepath"
)

// ResolveBinary ищет исполняемый файл рядом с текущим бинарником, затем в PATH.
// Порядок важен для Windows, где xray.exe часто кладут рядом с Fluffify.exe,
// не добавляя каталог в PATH.
func ResolveBinary(base string) string {
	name := ExeName(base)
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return name
}
