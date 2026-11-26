//go:build !windows

package autostart

import "testing"

func TestUnitContentPointsAtBinary(t *testing.T) {
	unit := unitContent("/usr/bin/xray-tui")
	if !unitMatchesExe(unit, "/usr/bin/xray-tui") {
		t.Fatal("юнит должен ссылаться на текущий бинарник")
	}
	if unitMatchesExe(unit, "/home/lypka/xray-tui/xray-tui") {
		t.Fatal("юнит из старого каталога должен требовать перезаписи")
	}
}

// Регрессия: юнит от прежней сборки нельзя оставлять как есть — иначе systemd
// поднимает устаревший демон, и правки в приложении не видны.
func TestUnitMatchesExeDetectsStale(t *testing.T) {
	stale := `[Unit]
Description=Xray TUI Background Daemon

[Service]
ExecStart=%h/xray-tui/xray-tui daemon
`
	if unitMatchesExe(stale, "/usr/bin/xray-tui") {
		t.Fatal("устаревший путь не должен считаться совпадением")
	}
	fresh := unitContent("/usr/bin/xray-tui")
	if !unitMatchesExe(fresh, "/usr/bin/xray-tui") {
		t.Fatal("свежий юнит должен совпадать")
	}
	if unitMatchesExe("", "/usr/bin/xray-tui") {
		t.Fatal("пустой юнит не совпадает")
	}
}
