//go:build !windows

package osutil

import (
	"os"
	"syscall"
)

// Terminate просит процесс завершиться штатно (SIGTERM).
func Terminate(p *os.Process) error {
	return p.Signal(syscall.SIGTERM)
}
