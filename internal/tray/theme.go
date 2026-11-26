package tray

// themeKind — тема оформления, влияющая на цвета иконки трея.
type themeKind int

const (
	themeUnknown themeKind = iota
	themeDark
	themeLight
)

// Цвета иконки по состояниям инбаунда.
// Тёмная тема: неактивно — тёмно-серый (видно на тёмной панели), активно — белый.
// Светлая тема: неактивно — светло-серый, активно — чёрный.
const (
	darkInactive    = "#8a8a91"
	lightInactive   = "#b9b9c0"
	neutralInactive = "#8a8a91"

	darkActive  = "#ffffff"
	lightActive = "#1f1f1f"
)

// errorIconColor — цвет иконки в состоянии ошибки (нет связи с демоном,
// отказ сервера). Строго ярко-красный.
const errorIconColor = "#ff0000"

// themeIconColors возвращает цвета неактивного и активного состояний.
// Пустая строка в active означает «тема неизвестна, красить в акцент».
func themeIconColors(theme themeKind) (inactive, active string) {
	switch theme {
	case themeDark:
		return darkInactive, darkActive
	case themeLight:
		return lightInactive, lightActive
	default:
		return neutralInactive, ""
	}
}
