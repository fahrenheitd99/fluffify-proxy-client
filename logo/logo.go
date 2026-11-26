// Package logo хранит встроенную графику приложения.
package logo

import _ "embed"

// SymbolicPNG — белая маска символической иконки (256×256, значим только альфа-канал).
// Трей перекрашивает её в цвет состояния.
//
//go:embed fluffify-symbolic.png
var SymbolicPNG []byte

// AppICO — многоразмерная .ico (16…256) для ярлыков и ресурса .exe.
//
//go:embed fluffify.ico
var AppICO []byte

// SymbolicWhiteICO и SymbolicBlackICO — символическая иконка трея для тёмной
// и светлой темы Windows соответственно.
//
//go:embed fluffify-symbolic-white.ico
var SymbolicWhiteICO []byte

//go:embed fluffify-symbolic-black.ico
var SymbolicBlackICO []byte
