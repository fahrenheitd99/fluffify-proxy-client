//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"xray-tui/internal/osutil"
)

// runTUNSetup проверяет, что у процесса демона есть права на TUN, и
// подсказывает, как их выдать. Ядро теперь встроено, поэтому setcap на xray
// больше не нужен: права должен иметь наш бинарник.
func runTUNSetup() error {
	if osutil.HasTunPermission() {
		fmt.Println("Права на TUN есть (CAP_NET_ADMIN у процесса демона).")
		return nil
	}

	bin := "/usr/bin/xray-tui"
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			bin = resolved
		}
	}

	fmt.Println("У демона нет прав на TUN. Варианты:")
	fmt.Println("  1) пакетный юнит systemd (рекомендуется): в нём уже заданы")
	fmt.Println("     AmbientCapabilities=CAP_NET_ADMIN,CAP_NET_RAW, поэтому достаточно:")
	fmt.Println("       systemctl --user restart xray-tui.service")
	fmt.Println("  2) выдать права бинарнику напрямую:")
	fmt.Printf("       sudo setcap cap_net_admin,cap_net_raw+ep %s\n", bin)
	fmt.Println("     и перезапустить демон: systemctl --user restart xray-tui.service")
	return nil
}
