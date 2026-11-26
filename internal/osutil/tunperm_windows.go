//go:build windows

package osutil

// HasTunPermission сообщает, может ли текущий процесс создать TUN-адаптер.
// В Windows это право есть только у администратора.
func HasTunPermission() bool {
	return IsElevated()
}
