//go:build !windows

package osutil

// HideConsole — no-op: на Unix консоль принадлежит терминалу, а не процессу.
func HideConsole() {}

// HideConsoleIfOwned — no-op по той же причине.
func HideConsoleIfOwned() {}
