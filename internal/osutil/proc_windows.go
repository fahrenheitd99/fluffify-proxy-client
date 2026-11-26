//go:build windows

package osutil

import "os"

// Terminate завершает процесс принудительно: SIGTERM в Windows не доставляется.
func Terminate(p *os.Process) error {
	return p.Kill()
}
