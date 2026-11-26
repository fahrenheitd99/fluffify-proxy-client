//go:build !windows

package osutil

// TunRequiresAdmin: в Linux права выдаются самому бинарнику xray
// (xray-tui tun-setup → setcap cap_net_admin), поэтому демон может быть обычным.
func TunRequiresAdmin() bool { return false }
