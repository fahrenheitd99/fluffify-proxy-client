//go:build windows

package main

import (
	"fmt"

	"xray-tui/internal/osutil"
)

func runTUNSetup() error {
	if osutil.HasTunPermission() {
		fmt.Println("Права администратора есть — TUN доступен (ядро и wintun.dll уже извлечены в %LocalAppData%\\Fluffify\\bin).")
		return nil
	}
	return fmt.Errorf("TUN в Windows требует прав администратора: запусти %s от имени администратора "+
		"или выполни %s restart-admin", osutil.SelfName(), osutil.SelfName())
}
