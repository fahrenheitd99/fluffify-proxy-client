//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"xray-tui/internal/osutil"
	"xray-tui/internal/tui"
)

func runTUNSetup() error {
	xrayPath, err := exec.LookPath(osutil.ExeName("xray"))
	if err != nil {
		return errors.New("бинарник xray не найден в PATH")
	}
	resolved, err := filepath.EvalSymlinks(xrayPath)
	if err == nil {
		xrayPath = resolved
	}

	fmt.Printf("Выдаю cap_net_admin,cap_net_raw для %s...\n", xrayPath)
	var cmd *exec.Cmd
	if _, err := exec.LookPath("pkexec"); err == nil && tui.HasGUI() {
		cmd = exec.Command("pkexec", "setcap", "cap_net_admin,cap_net_raw+ep", xrayPath)
	} else {
		cmd = exec.Command("sudo", "setcap", "cap_net_admin,cap_net_raw+ep", xrayPath)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("не удалось выдать права: %w", err)
	}
	fmt.Println("Готово. Внешние скрипты и sudoers не нужны — TUN работает напрямую через Xray.")
	return nil
}
