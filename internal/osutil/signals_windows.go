//go:build windows

package osutil

import "os"

// ShutdownSignals: на Windows доставляется только Ctrl+C (и Ctrl+Break).
// SIGTERM существует как константа, но системой не генерируется.
func ShutdownSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}
