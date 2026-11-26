//go:build !windows

package osutil

// EnableVirtualTerminal — no-op: на Unix терминал и так понимает ANSI и UTF-8.
func EnableVirtualTerminal() {}
