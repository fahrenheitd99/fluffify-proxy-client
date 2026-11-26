//go:build windows

package tray

import "xray-tui/internal/config"

// trayAccentPath: на Windows аналога kdeglobals нет, живое обновление не нужно.
func trayAccentPath() string { return "" }

// trayAccent берёт акцент из настроек приложения — их задаёт пользователь в TUI.
func trayAccent() string {
	store, err := config.Load()
	if err != nil {
		return defaultAccent
	}
	if hex, ok := accentHexByName[store.Settings.AccentColor]; ok {
		return hex
	}
	return defaultAccent
}

const defaultAccent = "#3daee9"

// accentHexByName повторяет тёмную палитру TUI (internal/tui/theme.go).
var accentHexByName = map[string]string{
	config.AccentRed:    "#ff3b30",
	config.AccentOrange: "#ff9500",
	config.AccentYellow: "#ffd60a",
	config.AccentGreen:  "#30d158",
	config.AccentCyan:   "#32d2f2",
	config.AccentBlue:   "#0a84ff",
	config.AccentViolet: "#bf5af2",
	config.AccentPink:   "#ff7eb6",
	config.AccentBrown:  "#b07d56",
	config.AccentWhite:  "#f4f4f6",
	config.AccentBlack:  "#141418",
}
