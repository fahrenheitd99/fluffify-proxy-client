//go:build windows

package osutil

// TunRequiresAdmin: в Windows виртуальный адаптер создаёт только процесс
// с правами администратора.
func TunRequiresAdmin() bool { return true }
