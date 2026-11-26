//go:build windows

package ping

// icmpArgs — аргументы Windows ping: один пакет, таймаут 2000 мс.
// Привязки к интерфейсу (-I) в Windows ping нет.
func icmpArgs(ip, _ string) []string {
	return []string{"-n", "1", "-w", "2000", ip}
}
