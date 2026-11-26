//go:build windows

package osutil

import (
	"errors"

	"golang.org/x/sys/windows"
)

// TryLock захватывает именованный мьютекс: он освобождается ядром при падении
// процесса, в отличие от файла-заглушки.
func TryLock(name string) (unlock func(), ok bool) {
	ptr, err := windows.UTF16PtrFromString(`Local\` + name)
	if err != nil {
		return nil, false
	}
	h, err := windows.CreateMutex(nil, false, ptr)
	if h == 0 {
		return nil, false
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || err != nil {
		_ = windows.CloseHandle(h)
		return nil, false
	}
	return func() { _ = windows.CloseHandle(h) }, true
}
