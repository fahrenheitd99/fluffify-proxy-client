//go:build !windows

package osutil

import "os"

// TUNDeviceReady сообщает, зарегистрирован ли в ядре драйвер TUN
// (/sys/class/misc/tun появляется только при загруженном модуле tun).
// Нода /dev/net/tun не показатель: systemd создаёт её статически и без
// модуля, а open такой ноды даёт ENODEV «no such device».
func TUNDeviceReady() bool {
	_, err := os.Stat("/sys/class/misc/tun")
	return err == nil
}
