package osutil

import (
	"os"
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
