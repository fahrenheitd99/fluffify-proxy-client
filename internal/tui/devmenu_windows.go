//go:build windows

package tui

// openDaemonJournal недоступен на Windows (journalctl — часть systemd).
func openDaemonJournal() {}

// tunProbe — на Windows проверка готовности к TUN не выполняется.
func tunProbe() tunProbeMsg { return tunProbeMsg{known: false} }

// tunReadinessReport — заглушка: отчёт есть только на Linux.
func tunReadinessReport() string { return "" }
