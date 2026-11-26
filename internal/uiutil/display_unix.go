//go:build !windows

package uiutil

// Display на Unix ничего не меняет: флаги стран рисуются нормально.
func Display(s string) string {
	return s
}
