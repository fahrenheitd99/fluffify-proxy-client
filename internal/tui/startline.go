package tui

import "strings"

// buildStartLine собирает командную строку для cmd.exe вручную.
//
// os/exec квотирует только аргументы с пробелами, поэтому "Fluffify"
// уходил в start без кавычек — и тот принимал его за имя программы,
// пытаясь запустить Fluffify.exe. Здесь кавычки расставлены явно:
// "Fluffify" — заголовок, /D — рабочая папка, дальше сам бинарник.
func buildStartLine(comspec, dir, exe string) string {
	if strings.HasSuffix(dir, `\`) {
		// "C:\" перед закрывающей кавычкой cmd читает как escape-последовательность.
		dir += "."
	}
	return `"` + comspec + `" /c start "Fluffify" /D "` + dir + `" "` + exe + `" tui`
}
