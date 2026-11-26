//go:build !windows

package ping

import (
	"bufio"
	"os"
	"strings"
)

// PhysicalIface находит активный физический интерфейс в /proc/net/route, исключая TUN и lo.
func PhysicalIface(exclude string) string {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer f.Close()

	var fallback string
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		iface, dest, gw := fields[0], fields[1], fields[2]
		if iface == "lo" || (exclude != "" && iface == exclude) {
			continue
		}
		if dest == "00000000" {
			return iface
		}
		if fallback == "" && gw != "00000000" {
			fallback = iface
		}
	}
	return fallback
}
