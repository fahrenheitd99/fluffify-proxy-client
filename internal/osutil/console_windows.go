//go:build windows

package osutil

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	kernel32              = windows.NewLazySystemDLL("kernel32.dll")
	getConsoleWindow      = kernel32.NewProc("GetConsoleWindow")
	getConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
	showWindow            = user32.NewProc("ShowWindow")
)

// HideConsole прячет консольное окно процесса. Нужно для трея, который
// автозапуск стартует из реестра: консольный бинарник иначе оставляет окно.
func HideConsole() {
	hwnd, _, _ := getConsoleWindow.Call()
	if hwnd != 0 {
		showWindow.Call(hwnd, windows.SW_HIDE)
	}
}

// HideConsoleIfOwned прячет консоль только если процесс — единственный её
// владелец, то есть программу запустили двойным кликом, а не из терминала.
func HideConsoleIfOwned() {
	var pids [2]uint32
	n, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), 2)
	if n == 1 {
		HideConsole()
	}
}
