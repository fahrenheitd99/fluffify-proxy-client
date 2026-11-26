//go:build windows

package osutil

import "golang.org/x/sys/windows"

// EnableVirtualTerminal включает обработку ANSI-последовательностей и UTF-8
// в текущей консоли. Без этого старый conhost (особенно в сессии администратора)
// выводит кадры TUI «кашей» и кракозябрами вместо иконок.
func EnableVirtualTerminal() {
	// Кодировка консоли: 65001 = UTF-8.
	_ = windows.SetConsoleOutputCP(65001)
	_ = windows.SetConsoleCP(65001)

	for _, std := range []uint32{windows.STD_OUTPUT_HANDLE, windows.STD_ERROR_HANDLE} {
		h, err := windows.GetStdHandle(std)
		if err != nil || h == 0 || h == windows.InvalidHandle {
			continue
		}
		var mode uint32
		if err := windows.GetConsoleMode(h, &mode); err != nil {
			continue // вывод перенаправлен — это не консоль
		}
		mode |= windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
		_ = windows.SetConsoleMode(h, mode)
	}
}
