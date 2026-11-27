//go:build windows

package netcheck

import (
	"net"
	"strings"
)

// defaultIface на Windows: первый подходящий активный интерфейс с IPv4
// (эвристика — dev-инструмент, точного маршрута не требует).
func defaultIface() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || !suitable(ifc.Name) {
			continue
		}
		if ifaceIPv4(ifc.Name) != nil {
			return ifc.Name
		}
	}
	return ""
}

// suitable отсекает loopback, наш TUN и виртуальные адаптеры.
func suitable(name string) bool {
	l := strings.ToLower(name)
	if l == "lo" || strings.Contains(l, "loopback") || strings.HasPrefix(l, "xray") {
		return false
	}
	for _, p := range []string{"tun", "tap", "utun", "wg", "docker", "veth", "tailscale", "virtual", "vmware", "hyper-v", "bluetooth"} {
		if strings.Contains(l, p) {
			return false
		}
	}
	return true
}

// ifaceIPv4 — первый IPv4-адрес интерфейса (для LocalAddr bind'а).
func ifaceIPv4(name string) net.IP {
	ifc, err := net.InterfaceByName(name)
	if err != nil {
		return nil
	}
	addrs, err := ifc.Addrs()
	if err != nil {
		return nil
	}
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
			return ipn.IP.To4()
		}
	}
	return nil
}
