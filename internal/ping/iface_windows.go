//go:build windows

package ping

import (
	"net"
	"strings"
)

// PhysicalIface выбирает интерфейс для обхода TUN: поднятый, не loopback,
// с адресом IPv4 и без признаков tun/wintun в имени или описании.
func PhysicalIface(exclude string) string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	var fallback string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		lower := strings.ToLower(iface.Name)
		if lower == "lo" || (exclude != "" && iface.Name == exclude) {
			continue
		}
		if strings.Contains(lower, "wintun") || strings.Contains(lower, "xray") || strings.Contains(lower, "tun") {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		hasIPv4 := false
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if ok && ipnet.IP.To4() != nil {
				hasIPv4 = true
				break
			}
		}
		if !hasIPv4 {
			continue
		}
		if fallback == "" {
			fallback = iface.Name
		}
	}
	return fallback
}
