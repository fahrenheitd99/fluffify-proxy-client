//go:build windows

package main

import (
	"fmt"

	"xray-tui/internal/osutil"
)

func runTUNSetup() error {
	if osutil.IsElevated() {
		fmt.Println("Права администратора уже есть — TUN доступен (wintun.dll лежит рядом с xray.exe).")
		return nil
	}
	return fmt.Errorf("TUN в Windows требует прав администратора: запусти %s от имени администратора "+
		"или выполни %s restart-admin", osutil.SelfName(), osutil.SelfName())
}
