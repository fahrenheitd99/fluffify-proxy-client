//go:build linux

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"xray-tui/internal/osutil"
)

// runTUNSetup проверяет права на TUN и подсказывает, как их выдать.
// Ядро — отдельный процесс xray, который спавнит демон: права нужны самому
// xray. Демон передаёт их ребёнку через AmbientCapabilities юнита systemd,
// либо их дают setcap'ом на бинарнике xray.
func runTUNSetup() error {
	if osutil.HasTunPermission() {
		fmt.Println("Права на TUN есть (CAP_NET_ADMIN у текущего процесса).")
		return nil
	}

	fmt.Println("Прав на TUN нет. Ядро — отдельный процесс xray (его запускает демон), ему и нужны права. Варианты:")
	fmt.Println("  1) юнит systemd демона с AmbientCapabilities (пакетный или автозапуск):")
	fmt.Println("       systemctl --user restart xray-tui.service")
	fmt.Println("  2) выдать права самому бинарнику ядра:")

	xrayPath := osutil.ResolveBinary("xray")
	if !filepath.IsAbs(xrayPath) {
		fmt.Println("     ядро xray не найдено ни рядом с приложением, ни в PATH —")
		fmt.Println("     поставьте пакет xray или положите бинарник рядом с Fluffify.")
		return nil
	}
	if resolved, err := filepath.EvalSymlinks(xrayPath); err == nil {
		if _, err := os.Stat(resolved); err == nil {
			xrayPath = resolved
		}
	}
	fmt.Printf("       sudo setcap cap_net_admin,cap_net_raw+ep %s\n", xrayPath)
	return nil
}
