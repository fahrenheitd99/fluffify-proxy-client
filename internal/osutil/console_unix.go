//go:build !windows

package osutil

import "os/exec"

// HideConsole — no-op: на Unix консоль принадлежит терминалу, а не процессу.
func HideConsole() {}

// HideConsoleIfOwned — no-op по той же причине.
func HideConsoleIfOwned() {}

// HideChildConsole — no-op: на Unix дочерние процессы не создают окон.
func HideChildConsole(*exec.Cmd) {}
