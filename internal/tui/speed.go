package tui

import (
	"fmt"
	"strings"
	"time"

	"xray-tui/internal/config"
)

type netSpeedState struct {
	iface  string
	prevRx uint64
	prevTx uint64
	prevAt time.Time
	rxRate uint64
	txRate uint64
}

func (s *netSpeedState) reset() {
	*s = netSpeedState{}
}

func (s *netSpeedState) update(mode, tunName string) {
	iface, rx, tx, ok := readActiveNetBytes(mode, tunName)
	if !ok {
		s.rxRate = 0
		s.txRate = 0
		return
	}

	now := time.Now()
	if s.iface != iface || s.prevAt.IsZero() {
		s.iface = iface
		s.prevRx = rx
		s.prevTx = tx
		s.prevAt = now
		s.rxRate = 0
		s.txRate = 0
		return
	}

	dt := now.Sub(s.prevAt).Seconds()
	if dt >= 0.2 {
		if rx >= s.prevRx {
			s.rxRate = uint64(float64(rx-s.prevRx) / dt)
		} else {
			s.rxRate = 0
		}
		if tx >= s.prevTx {
			s.txRate = uint64(float64(tx-s.prevTx) / dt)
		} else {
			s.txRate = 0
		}
		s.prevRx = rx
		s.prevTx = tx
		s.prevAt = now
	}
}

func readActiveNetBytes(mode, tunName string) (string, uint64, uint64, bool) {
	stats := readNetStats()
	if len(stats) == 0 {
		return "", 0, 0, false
	}

	if mode == config.ModeTUN {
		if tunName == "" {
			tunName = "xray0"
		}
		if v, ok := stats[tunName]; ok {
			return tunName, v[0], v[1], true
		}
	}

	if defIface := defaultRouteIface(tunName); defIface != "" {
		if v, ok := stats[defIface]; ok {
			return defIface, v[0], v[1], true
		}
	}

	var bestIface string
	var bestTotal uint64
	for name, v := range stats {
		if isIgnoredIface(name, tunName) {
			continue
		}
		if total := v[0] + v[1]; bestIface == "" || total > bestTotal {
			bestIface = name
			bestTotal = total
		}
	}
	if bestIface != "" {
		v := stats[bestIface]
		return bestIface, v[0], v[1], true
	}
	return "", 0, 0, false
}

func isIgnoredIface(name, exclude string) bool {
	lower := strings.ToLower(name)
	if lower == "lo" || (exclude != "" && name == exclude) {
		return true
	}
	for _, p := range []string{"docker", "br-", "veth", "virbr", "tun", "xray", "wintun", "loopback"} {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func formatSpeed(bps uint64) string {
	switch {
	case bps < 1024:
		return fmt.Sprintf("%d B/s", bps)
	case bps < 1024*1024:
		return fmt.Sprintf("%.1f KB/s", float64(bps)/1024)
	case bps < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB/s", float64(bps)/(1024*1024))
	default:
		return fmt.Sprintf("%.2f GB/s", float64(bps)/(1024*1024*1024))
	}
}
