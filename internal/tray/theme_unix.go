//go:build !windows

package tray

// trayTheme: на Unix цвет иконки определяется акцентным цветом KDE.
func trayTheme() themeKind { return themeUnknown }
