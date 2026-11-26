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

// EnsureWintunDLL гарантирует, что драйвер TUN (wintun.dll) загрузится в
// процессе, где бы ни лежал exe:
//
//  1. dll рядом с exe — найдётся через LOAD_LIBRARY_SEARCH_APPLICATION_DIR;
//  2. иначе извлекаем встроенную копию в скрытую папку пользователя
//     %LocalAppData%\Fluffify\wintun.dll (0755/0644) — не сорим рядом с exe;
//  3. сообщаем Windows, где искать: AddDllDirectory добавляет папку в
//     поиск, SetDllDirectory задаёт её для устаревшего порядка, а главный
//     механизм — предзагрузка по абсолютному пути: модуль с именем
//     wintun.dll остаётся в процессе, и LoadLibraryEx у wintun вернёт
//     уже загруженную копию.
//
// Извлечённый файл не удаляется на выходе: он переиспользуется при
// следующих запусках.
func EnsureWintunDLL() {
	exe, err := os.Executable()
	if err != nil {
		return
	}

	// 1. Рядом с exe уже есть — wintun найдёт её сам.
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "wintun.dll")); err == nil {
		return
	}

	// 2. Скрытая папка пользователя: %LocalAppData%\Fluffify
	cache, err := os.UserCacheDir() // Windows: %LocalAppData%
	if err != nil {
		return
	}
	dir := filepath.Join(cache, "Fluffify")
	target := filepath.Join(dir, "wintun.dll")

	if info, err := os.Stat(target); err != nil || info.Size() != int64(len(wintunDLL)) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return
		}
		if err := os.WriteFile(target, wintunDLL, 0o644); err != nil {
			return
		}
	}

	// 3. Сообщаем Windows, где искать драйвер.
	if ptr, err := windows.UTF16PtrFromString(dir); err == nil {
		// AddDllDirectory — современный способ (список каталогов);
		// SetDllDirectory задаёт единственный каталог устаревшего порядка.
		if _, err := windows.AddDllDirectory(ptr); err != nil {
			_ = windows.SetDllDirectory(dir)
		}
	}

	// Предзагрузка по абсолютному пути: модуль «wintun.dll» остаётся в
	// процессе, и LoadLibraryEx у wintun вернёт эту копию. Хендл не
	// освобождаем.
	if handle, err := windows.LoadLibrary(target); err == nil {
		wintunModule = handle
	}
}
