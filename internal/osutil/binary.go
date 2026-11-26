package osutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// SelfName — имя текущего исполняемого файла (Fluffify.exe в Windows-сборке).
func SelfName() string {
	if exe, err := os.Executable(); err == nil {
		if base := filepath.Base(exe); base != "" && base != "." {
			return base
		}
	}
	if runtime.GOOS == "windows" {
		return "Fluffify.exe"
	}
	return "xray-tui"
}

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
