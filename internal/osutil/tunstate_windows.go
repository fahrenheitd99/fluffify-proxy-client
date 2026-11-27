//go:build windows

package osutil

// TUNDeviceReady — в Windows драйвер wintun.dll поставляется вместе с
// приложением, отдельного модуля ядра нет.
func TUNDeviceReady() bool { return true }
