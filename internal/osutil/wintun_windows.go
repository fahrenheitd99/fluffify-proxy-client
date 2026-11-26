//go:build windows

package osutil

import (
	_ "embed"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

//go:embed assets/wintun.dll
var wintunDLL []byte

// загрузка DLL — обычная (без LOAD_LIBRARY_SEARCH_*), модуль остаётся
// в процессе до завершения работы.
var wintunModule windows.Handle

// EnsureWintunDLL гарантирует, что wintun.dll загрузится в процессе демона:
// ядро Xray ищет её через LoadLibraryEx только в каталоге exe и в System32,
// поэтому «вытащенный на рабочий стол» бинарник без предзагрузки TUN не поднял бы.
//
// Порядок:
//  1. wintun.dll лежит рядом с exe — найдётся сама;
//  2. иначе берём копию из %LOCALAPPDATA%\xray-tui (при необходимости
//     распаковываем встроенную) и предзагружаем её по абсолютному пути:
//     модуль с именем wintun.dll окажется загруженным в процесс, и
//     LoadLibraryEx у wintun вернёт уже загруженную копию.
func EnsureWintunDLL() {
	if exe, err := os.Executable(); err == nil {
		if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "wintun.dll")); err == nil {
			return
		}
	}

	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		return
	}
	dest := filepath.Join(base, "xray-tui", "wintun.dll")

	if info, err := os.Stat(dest); err != nil || info.Size() != int64(len(wintunDLL)) {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return
		}
		if err := os.WriteFile(dest, wintunDLL, 0o644); err != nil {
			return
		}
	}

	// Держим модуль загруженным: FreeLibrary не вызываем намеренно.
	if handle, err := windows.LoadLibrary(dest); err == nil {
		wintunModule = handle
	}
}
