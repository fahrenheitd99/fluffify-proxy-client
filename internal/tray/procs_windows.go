//go:build windows

package tray

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"xray-tui/internal/osutil"
)

// terminateSiblingProcs завершает остальные процессы этого бинарника.
// taskkill по имени образа задел бы и текущий процесс, поэтому свой PID исключаем.
func terminateSiblingProcs() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	name := filepath.Base(exe)
	if name == "" || name == "." {
		name = osutil.SelfName()
	}
	kill := exec.Command("taskkill", "/F", "/IM", name,
		"/FI", "PID ne "+strconv.Itoa(os.Getpid()))
	osutil.HideChildConsole(kill)
	_ = kill.Run()
}
