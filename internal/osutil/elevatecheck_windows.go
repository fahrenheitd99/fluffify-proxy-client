//go:build windows

package osutil

// SelfElevateDaemon перезапускает текущий процесс демона с правами
// администратора, если он был запущен без них. true — перезапуск запущен и
// вызывающий должен завершиться; false — права уже есть либо пользователь
// отклонил UAC (тогда можно продолжить работу без прав: прокси работает,
// TUN попросит права позже).
func SelfElevateDaemon() bool {
	if HasTunPermission() {
		return false
	}
	if err := RelaunchElevated("daemon-hidden"); err != nil {
		return false
	}
	return true
}
