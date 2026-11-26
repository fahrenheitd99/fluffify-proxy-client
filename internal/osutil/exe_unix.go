//go:build !windows

package osutil

// ExeName возвращает имя исполняемого файла для текущей платформы.
func ExeName(base string) string { return base }
