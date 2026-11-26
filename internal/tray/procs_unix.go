//go:build !windows

package tray

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"xray-tui/internal/osutil"
)

// terminateSiblingProcs завершает остальные процессы этого бинарника:
// ручной демон и открытые окна TUI.
func terminateSiblingProcs() {
	myPID := os.Getpid()
	exe, _ := os.Executable()
	selfBase := filepath.Base(exe)
	if selfBase == "" || selfBase == "." {
		selfBase = osutil.SelfName()
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 1 || pid == myPID {
			continue
		}

		matched := false
		if target, err := os.Readlink(filepath.Join("/proc", e.Name(), "exe")); err == nil {
			clean := strings.TrimSuffix(target, " (deleted)")
			if (exe != "" && clean == exe) || filepath.Base(clean) == selfBase {
				matched = true
			}
		}
		if !matched {
			if raw, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline")); err == nil && len(raw) > 0 {
				parts := strings.Split(string(raw), "\x00")
				if len(parts) > 0 && filepath.Base(parts[0]) == selfBase {
					matched = true
				}
			}
		}

		if matched {
			_ = syscall.Kill(pid, syscall.SIGTERM)
		}
	}
}
