//go:build !windows

package ping

// icmpArgs — аргументы iputils/ping для одного замера, опционально через интерфейс.
func icmpArgs(ip, iface string) []string {
	args := []string{"-n", "-c", "1", "-W", "2"}
	if iface != "" {
		args = append(args, "-I", iface)
	}
	return append(args, ip)
}
