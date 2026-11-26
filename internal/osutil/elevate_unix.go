//go:build !windows

package osutil

import (
	"errors"
	"os"
)

// IsElevated: на Unix считаем root-права достаточными для TUN.
func IsElevated() bool {
	return os.Geteuid() == 0
}

// RelaunchElevated на Unix не реализован: права выдаются через su/sudo вручную.
func RelaunchElevated(_ ...string) error {
	return errors.New("перезапуск с повышением прав поддерживается только в Windows")
}
