//go:build !windows

package osutil

import (
	"os"
	"syscall"
)

// ShutdownSignals — сигналы, по которым демон завершает работу.
func ShutdownSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}
