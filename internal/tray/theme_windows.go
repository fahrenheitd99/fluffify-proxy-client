//go:build windows

package tray

import "golang.org/x/sys/windows/registry"

const personalizeKey = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`

// trayTheme читает AppsUseLightTheme из HKCU: 0 — тёмная тема, 1 — светлая.
// На тёмной теме нужна белая иконка, на светлой — чёрная.
func trayTheme() themeKind {
	k, err := registry.OpenKey(registry.CURRENT_USER, personalizeKey, registry.QUERY_VALUE)
	if err != nil {
		return themeUnknown
	}
	defer k.Close()

	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	if err != nil {
		return themeUnknown
	}
	if v == 0 {
		return themeDark
	}
	return themeLight
}
