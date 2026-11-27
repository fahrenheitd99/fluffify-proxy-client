//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"xray-tui/internal/osutil"
	"xray-tui/internal/tui"
)

const tunCaps = "cap_net_admin,cap_net_raw+ep"

// runTUNSetup готовит систему к TUN-режиму одной привилегированной командой:
//
//  1. модуль ядра tun (без него open("/dev/net/tun") даёт «no such device»:
//     ноду systemd создаёт статически, а драйвер не зарегистрирован) +
//     автозагрузка модуля при старте системы (/etc/modules-load.d/tun.conf);
//  2. файловые права ядру xray: systemd user-юнит не может выдать
//     CAP_NET_ADMIN демону, поэтому caps должны лежать на самом бинарнике.
//
// Права применяются через pkexec (в GUI) или sudo (в терминале).
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

	moduleMissing := !osutil.TUNDeviceReady()
	capsMissing := !xrayHasFileCaps(xrayPath)

	if !moduleMissing && !capsMissing {
		fmt.Printf("TUN готов: модуль ядра tun загружен, права на %s есть.\n", xrayPath)
		return nil
	}

	var parts []string
	if moduleMissing {
		parts = append(parts,
			"modprobe tun",
			"mkdir -p /etc/modules-load.d",
			"echo tun > /etc/modules-load.d/tun.conf",
		)
	}
	if capsMissing {
		parts = append(parts, "setcap "+tunCaps+" "+shQuote(xrayPath))
	}
	script := strings.Join(parts, " && ")

	fmt.Println("Нужны права администратора — подтверди запрос:")
	var cmd *exec.Cmd
	if _, err := exec.LookPath("pkexec"); err == nil && tui.HasGUI() {
		cmd = exec.Command("pkexec", "sh", "-c", script)
	} else {
		fmt.Println("pkexec недоступен — запускаю sudo (введи пароль в терминале):")
		cmd = exec.Command("sudo", "sh", "-c", script)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("не удалось применить настройки TUN: %w", err)
	}

	if moduleMissing && !osutil.TUNDeviceReady() {
		return fmt.Errorf("модуль ядра tun не загрузился — проверь, что в ядре есть поддержка TUN (CONFIG_TUN=m)")
	}
	if capsMissing && !xrayHasFileCaps(xrayPath) {
		return fmt.Errorf("setcap отработал, но права не видны на %s — перезапусти демон и повтори", xrayPath)
	}

	fmt.Println("Готово: модуль tun загружен (и будет загружаться при старте системы), права на ядро выданы.")
	fmt.Println("Перезапусти демон (выйди из приложения и запусти заново) и включи TUN.")
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

// shQuote экранирует путь для sh -c "...".
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
