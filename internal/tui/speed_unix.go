//go:build !windows

package tui

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// readNetStats читает счётчики байт по интерфейсам из /proc/net/dev.
func readNetStats() map[string][2]uint64 {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil
	}
	defer f.Close()

	res := make(map[string][2]uint64)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 9 {
			continue
		}
		rx, err1 := strconv.ParseUint(fields[0], 10, 64)
		tx, err2 := strconv.ParseUint(fields[8], 10, 64)
		if err1 == nil && err2 == nil {
			res[strings.TrimSpace(name)] = [2]uint64{rx, tx}
		}
	}
	return res
}

func defaultRouteIface(exclude string) string {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer f.Close()

	var bestIface string
	bestMetric := int(^uint(0) >> 1)

	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(sc.Text())
		if len(fields) < 8 {
			continue
		}
		iface, dest, mask := fields[0], fields[1], fields[7]
		if dest != "00000000" || mask != "00000000" {
			continue
		}
		if iface == "lo" || (exclude != "" && iface == exclude) {
			continue
		}
		metric, _ := strconv.Atoi(fields[6])
		if bestIface == "" || metric < bestMetric {
			bestIface = iface
			bestMetric = metric
		}
	}
	return bestIface
}
