//go:build windows

package tui

// devMenuLogItems — на Windows пункты «Логи ядра»/«Логи приложения»
// в dev-меню отсутствуют: журнал можно смотреть средствами системы,
// а dev-меню ограничено ядром и туннелем.
func devMenuLogItems() []string { return nil }

// devMenuLogValues — значения log-пунктов (не используются).
func devMenuLogValues(string) []string { return nil }

// devMenuOffset — лог-блок пуст, базовые пункты с нуля.
func devMenuOffset() int { return 0 }

// devMenuExtraTail — хвостовые пункты (TUN/journalctl) только на Linux.
func devMenuExtraTail() []string { return nil }

// devMenuExtraTailValues — значения хвостовых пунктов.
func (m model) devMenuExtraTailValues() []string { return nil }

// openDaemonJournal недоступен на Windows (journalctl — часть systemd).
func openDaemonJournal() {}

// tunProbe — на Windows проверка готовности к TUN не выполняется.
func tunProbe() tunProbeMsg { return tunProbeMsg{known: false} }

// tunReadinessReport — заглушка: отчёт есть только на Linux.
func tunReadinessReport() string { return "" }
