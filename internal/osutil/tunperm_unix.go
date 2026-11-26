//go:build !windows

package osutil

import (
	"os"
	"strconv"
	"strings"
)

// HasTunPermission сообщает, может ли текущий процесс открыть /dev/net/tun
// и настроить интерфейс: root либо эффективный CAP_NET_ADMIN.
func HasTunPermission() bool {
	if os.Geteuid() == 0 {
		return true
	}
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(status), "\n") {
		if value, ok := strings.CutPrefix(line, "CapEff:"); ok {
			mask, err := strconv.ParseUint(strings.TrimSpace(value), 16, 64)
			if err != nil {
				return false
			}
			return mask&(1<<12) != 0 // CAP_NET_ADMIN = 12
		}
	}
	return false
}
