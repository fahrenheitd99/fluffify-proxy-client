//go:build windows

package ipc

import (
	"os"
	"path/filepath"
)

// defaultSocketPath держит путь коротким: AF_UNIX в Windows ограничен 108 байтами.
func defaultSocketPath() string {
	dir := os.Getenv("LOCALAPPDATA")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "xray-tui", "daemon.sock")
}
