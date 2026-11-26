// Package about содержит встроенные в бинарник лицензии сторонних
// компонентов (ядро Xray-core и драйвер wintun): пользователь читает их
// в приложении, без внешних текстовых файлов рядом с exe.
package about

import (
	_ "embed"
	"sort"
	"strings"
)

//go:embed assets/xray-license.txt
var xrayLicense string

//go:embed assets/wintun-license.txt
var wintunLicense string

// Entry — один встроенный текст лицензии/документации.
type Entry struct {
	Name string
	Text string
}

// ProjectLicense — лицензия самого Fluffify.
const ProjectLicense = "GPL-3.0-or-later"

// XrayCoreVersion — версия ядра, встроенного в релизный exe
// (пакет Xray-windows-64.zip, из которого собран ассет xray.exe.gz).
const XrayCoreVersion = "26.11.9"

// ProjectName — название приложения (папка данных использует его же).
const ProjectName = "Fluffify"

// Entries возвращает тексты лицензий встроенных компонентов.
func Entries() []Entry {
	entries := []Entry{
		{Name: "Xray-core (Mozilla Public License 2.0)", Text: strings.TrimSpace(xrayLicense)},
		{Name: "wintun (MIT License)", Text: strings.TrimSpace(wintunLicense)},
	}
	return entries
}

// Combined возвращает все лицензии одним текстом — для флага `licenses`.
func Combined() string {
	entries := Entries()
	// Стабильный порядок: по имени.
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	var b strings.Builder
	for _, entry := range entries {
		b.WriteString("=== " + entry.Name + " ===\n")
		b.WriteString(strings.TrimRight(entry.Text, "\n"))
		b.WriteString("\n\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
