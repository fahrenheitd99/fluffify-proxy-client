//go:build !windows

package netcheck

import (
	"net"
	"os"
	"strings"
)

// defaultIface — имя интерфейса физического аплинка: default-маршрут
// с минимальной метрикой из /proc/net/route, исключая туннели и loopback.
// Если /proc/net/route недоступен — первый подходящий активный интерфейс.
func defaultIface() string {
	if data, err := os.ReadFile("/proc/net/route"); err == nil {
		best := ""
		bestMetric := -1
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if i == 0 {
				continue
			}
			f := strings.Fields(line)
			// iface destination gateway flags refcnt use metric mask
			if len(f) < 8 || f[1] != "00000000" {
				continue
			}
			name := f[0]
			if !suitable(name) {
				continue
			}
			m := parseDec(f[6])
			if best == "" || m < bestMetric {
				best = name
				bestMetric = m
			}
		}
		if best != "" {
			return best
		}
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp != 0 && suitable(ifc.Name) {
			return ifc.Name
		}
	}
	return ""
}

func parseDec(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return n
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// suitable отсекает loopback, наш TUN и распространённые виртуальные интерфейсы.
func suitable(name string) bool {
	if name == "lo" || name == "xray0" || name == "tun0" {
		return false
	}
	for _, p := range []string{"tun", "tap", "utun", "wg", "docker", "veth", "br-", "virbr", "tailscale"} {
		if strings.HasPrefix(name, p) {
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
