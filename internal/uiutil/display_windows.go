//go:build windows

package uiutil

// Display приводит строку к виду, который корректно рисует Windows Terminal:
// флаги стран заменяются тегами [XX].
func Display(s string) string {
	return replaceRegionalFlags(s)
}
