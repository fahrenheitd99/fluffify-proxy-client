//go:build !windows

package ipc

import (
	"fmt"
	"os"
	"path/filepath"
)

func defaultSocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "xray-tui", "daemon.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("xray-tui-%d", os.Getuid()), "daemon.sock")
}
