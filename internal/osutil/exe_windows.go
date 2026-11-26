//go:build windows

package osutil

import "strings"

// ExeName добавляет .exe, если расширение не указано явно.
func ExeName(base string) string {
	if base == "" || strings.HasSuffix(strings.ToLower(base), ".exe") {
		return base
	}
	return base + ".exe"
}
