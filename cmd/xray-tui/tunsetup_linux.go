//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"xray-tui/internal/osutil"
)

// runTUNSetup систему не меняет — только проверяет состояние и печатает
// разовые команды. Для TUN на Linux нужны:
//
//  1. загруженный модуль ядра tun: нода /dev/net/tun создаётся systemd
//     статически, а без модуля open() даёт «no such device»;
//  2. файловые права на бинарнике ядра xray: user-юнит systemd не может
//     выдать демону CAP_NET_ADMIN, а caps переживают перезагрузку.
func runTUNSetup() error {
	xrayPath := osutil.ResolveBinary("xray")
	if !filepath.IsAbs(xrayPath) {
		return fmt.Errorf("ядро xray не найдено ни в PATH, ни рядом с приложением")
	}
	if resolved, err := filepath.EvalSymlinks(xrayPath); err == nil {
		if _, err := os.Stat(resolved); err == nil {
			xrayPath = resolved
		}
	}

	var problems []string
	if _, err := os.Stat("/sys/class/misc/tun"); err != nil {
		problems = append(problems, "1) загрузить модуль ядра tun и включить его при старте системы:\n"+
			"       sudo modprobe tun\n"+
			"       echo tun | sudo tee /etc/modules-load.d/tun.conf")
	}
	if !xrayHasFileCaps(xrayPath) {
		problems = append(problems, "2) выдать права бинарнику ядра:\n"+
			"       sudo setcap cap_net_admin,cap_net_raw+ep "+xrayPath)
	}

	if len(problems) == 0 {
		fmt.Printf("TUN готов: модуль ядра tun загружен, права на %s есть.\n", xrayPath)
		return nil
	}

	fmt.Println("Для TUN нужно сделать разовые настройки системы (после этого перезапусти демон):")
	for _, p := range problems {
		fmt.Println(p)
	}
	return nil
}

// xrayHasFileCaps проверяет файловые права ядра через getcap.
func xrayHasFileCaps(path string) bool {
	if _, err := exec.LookPath("getcap"); err != nil {
		return false
	}
	out, err := exec.Command("getcap", path).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "cap_net_admin")
}
