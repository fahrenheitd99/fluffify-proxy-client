//go:build !windows

package tray

// buildTrayIcons рендерит символическую SVG в PNG трёх состояний.
// Цвет «включено» берётся из системного акцента KDE.
func buildTrayIcons(accentHex string) (offPNG, onPNG, errPNG []byte) {
	svgRaw := loadSymbolicSVG()

	if b := renderSVGToPNG(colorizeSVG(svgRaw, accentHex, 1.0)); len(b) > 0 {
		onPNG = b
	} else {
		onPNG = makeFallbackIconPNG("on")
	}

	if b := renderSVGToPNG(colorizeSVG(svgRaw, "#5c5c68", 1.0)); len(b) > 0 {
		offPNG = b
	} else {
		offPNG = makeFallbackIconPNG("off")
	}

	if b := renderSVGToPNG(colorizeSVG(svgRaw, errorIconColor, 1.0)); len(b) > 0 {
		errPNG = b
	} else {
		errPNG = makeFallbackIconPNG("err")
	}

	return offPNG, onPNG, errPNG
}
