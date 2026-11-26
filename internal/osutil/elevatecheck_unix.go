//go:build !windows

package osutil

// SelfElevateDaemon — no-op: в Linux права демону выдаёт systemd
// (AmbientCapabilities), самоподнятие не используется.
func SelfElevateDaemon() bool { return false }
