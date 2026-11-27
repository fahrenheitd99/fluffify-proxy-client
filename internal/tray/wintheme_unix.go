//go:build !windows

package tray

// applyMenuTheme — заглушка: на Linux контекстное меню (DBus/KDE)
// темится системными средствами, uxtheme-хак не нужен.
func applyMenuTheme() {}
