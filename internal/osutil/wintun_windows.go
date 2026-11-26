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

// wintunModule — хендл предзагруженной библиотеки. Освобождать его нельзя:
// модуль должен остаться загруженным, чтобы wintun нашёл его у себя.
var wintunModule windows.Handle

// EnsureWintunDLL гарантирует, что wintun.dll загрузится в процессе:
// ядро Xray ищет её через LoadLibraryEx только в каталоге exe и в System32,
// поэтому exe, «вытащенный из папки» без предзагрузки, TUN не поднял бы.
//
// Порядок:
//  1. wintun.dll лежит рядом с exe — найдётся сама;
//  2. иначе извлекаем встроенную копию рядом с exe (0644);
//  3. каталог exe недоступен для записи (Program Files) — извлекаем в
//     каталог пользователя (%LocalAppData%\xray-tui\wintun.dll) и
//     предзагружаем по абсолютному пути: модуль с именем wintun.dll
//     окажется в процессе, и LoadLibraryEx у wintun вернёт уже
//     загруженную копию (SetDllDirectory/AddDllDirectory тут не работают —
//     wintun использует LOAD_LIBRARY_SEARCH_*-флаги).
//
// Из каталога exe файл на выходе не удаляем: при следующем запуске он
// уже на месте и никуда не девается.
func EnsureWintunDLL() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	exeDir := filepath.Dir(exe)
	target := filepath.Join(exeDir, "wintun.dll")

	if _, err := os.Stat(target); err == nil {
		return // лежит рядом — найдётся через APPLICATION_DIR
	}

	// 2. Пробуем извлечь рядом с exe.
	if err := os.WriteFile(target, wintunDLL, 0o644); err == nil {
		return
	}

	// 3. Каталог exe только для чтения — кэш пользователя и предзагрузка.
	cache, err := os.UserCacheDir()
	if err != nil {
		return
	}
	cachePath := filepath.Join(cache, "xray-tui", "wintun.dll")

	if info, err := os.Stat(cachePath); err != nil || info.Size() != int64(len(wintunDLL)) {
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
			return
		}
		if err := os.WriteFile(cachePath, wintunDLL, 0o644); err != nil {
			return
		}
	}

	// Предзагрузка по абсолютному пути: имя модуля «wintun.dll» попадает в
	// список загруженных, поэтому LoadLibraryEx("wintun.dll", …) у wintun
	// вернёт эту копию. Хендл не освобождаем.
	if handle, err := windows.LoadLibrary(cachePath); err == nil {
		wintunModule = handle
	}
}
