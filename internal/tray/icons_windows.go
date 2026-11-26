//go:build windows

package tray

import "xray-tui/logo"

// buildTrayIcons собирает иконки состояний: неактивное и активное по теме
// Windows (тёмно-серый/белая на тёмной, светло-серый/чёрная на светлой)
// плюс «ошибка». systray принимает только .ico.
func buildTrayIcons(accentHex string) (offICO, onICO, errICO []byte) {
	theme := trayTheme()
	inactive, active := themeIconColors(theme)
	if active == "" {
		active = accentHex
	}

	on := tintICO(active)
	if ico := themeActiveICO(theme); len(ico) > 0 {
		on = ico
	}
	return tintICO(inactive), on, tintICO(errorIconColor)
}

// themeActiveICO — встроенная белая/чёрная иконка активного состояния.
func themeActiveICO(theme themeKind) []byte {
	switch theme {
	case themeDark:
		return logo.SymbolicWhiteICO
	case themeLight:
		return logo.SymbolicBlackICO
	default:
		return nil
	}
}
