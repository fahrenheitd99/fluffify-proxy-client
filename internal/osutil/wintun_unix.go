//go:build !windows

package osutil

// EnsureWintunDLL — no-op: wintun нужен только в Windows.
func EnsureWintunDLL() {}
