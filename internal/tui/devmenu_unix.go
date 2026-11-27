//go:build !windows

package tui

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	"xray-tui/internal/autostart"
	"xray-tui/internal/devterm"
	"xray-tui/internal/osutil"
)

// devMenuLogItems — два первых пункта dev-меню (журналы). Только Unix:
// на Windows журнал ядра/приложения можно смотреть средствами системы,
// а PS-окна с tail решили не показывать.
func devMenuLogItems() []string {
	return []string{"Логи ядра (info) — вкл/выкл", "Логи приложения"}
}

// devMenuLogValues — значения для log-пунктов.
func devMenuLogValues(devLog string) []string {
	return []string{devLog, "открыть →"}
}

// devMenuOffset — размер log-блока перед базовыми пунктами.
func devMenuOffset() int { return len(devMenuLogItems()) }

// devMenuExtraTail — хвостовые пункты (Linux-only): TUN и journalctl.
func devMenuExtraTail() []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	return []string{"Готовность к TUN", "Журнал демона (journalctl)"}
}

// devMenuExtraTailValues — значения хвостовых пунктов.
func (m model) devMenuExtraTailValues() []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	tunReady := "?"
	if m.tunReadyKnown {
		tunReady = onOffLabel(m.tunReady)
	}
	return []string{tunReady, "открыть →"}
}

// openDaemonJournal открывает журнал демона: системный юнит из пакета
// или пользовательский fallback.
func openDaemonJournal() {
	unit := "xray-tui.service"
	prefix := "journalctl --user -n 300 -f -u "
	if autostart.SystemDaemonUnitAvailable() {
		unit = "fluffify-daemon@" + currentUserNameSafe() + ".service"
		prefix = "journalctl -n 300 -f -u "
	}
	_ = devterm.Terminal(prefix+unit, false)
}

// tunProbe — быстрая проверка готовности к TUN.
func tunProbe() tunProbeMsg {
	if runtime.GOOS != "linux" {
		return tunProbeMsg{known: false}
	}
	if _, err := os.Stat("/dev/net/tun"); err != nil {
		return tunProbeMsg{known: true, ok: false}
	}
	known, ok := xrayCapsState()
	return tunProbeMsg{known: known, ok: ok}
}

func readOneLine(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
}

// xrayCapsState проверяет file capabilities у текущего бинарника xray.
func xrayCapsState() (known, ok bool) {
	bin, err := exec.LookPath("getcap")
	if err != nil {
		return false, false
	}
	out, err := exec.Command(bin, resolveXrayBin()).CombinedOutput()
	if err != nil {
		return false, false
	}
	return true, strings.Contains(string(out), "cap_net_admin")
}

func polkitRuleInstalled() bool {
	_, err := os.Stat("/usr/share/polkit-1/rules.d/50-fluffify-daemon.rules")
	return err == nil
}

func currentUserNameSafe() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("LOGNAME"); u != "" {
		return u
	}
	out, err := exec.Command("id", "-un").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// resolveXrayBin — путь к бинарнику ядра (системный или из PATH).
func resolveXrayBin() string {
	return osutil.ResolveBinary("xray")
}

// tunReadinessReport — сводка готовности системы к TUN-режиму.
func tunReadinessReport() string {
	var b strings.Builder
	check := func(ok bool, okTxt, badTxt string) {
		mark := "[✗] "
		if ok {
			mark = "[✓] "
		}
		b.WriteString(mark + okTxt)
		if !ok && badTxt != "" {
			b.WriteString(" — " + badTxt)
		}
		b.WriteString("\n")
	}

	_, statErr := os.Stat("/dev/net/tun")
	check(statErr == nil, "устройство /dev/net/tun", "модуль tun не загружен (sudo modprobe tun)")

	capsKnown, capsOK := xrayCapsState()
	switch {
	case !capsKnown:
		b.WriteString("[?] права на xray: не удалось определить\n")
	case capsOK:
		b.WriteString("[✓] права на xray: capabilities есть\n")
	default:
		b.WriteString("[✗] права на xray: capabilities нет — DNS в TUN может не заработать без setcap\n")
	}

	unitOK := autostart.SystemDaemonUnitAvailable()
	check(unitOK, "системный юнит fluffify-daemon@.service установлен", "автозапуск через пользовательский юнит")

	ruleOK := polkitRuleInstalled()
	check(ruleOK, "polkit-правило resolve1 установлено", "демон будет запрашивать пароль при смене DNS")

	ipFwd := readOneLine("/proc/sys/net/ipv4/ip_forward")
	check(ipFwd == "1", "IP forwarding включён", "выключен — Xray включит его сам при старте TUN, это не ошибка")

	b.WriteString("\nПодсказка: без системного пакета включите автозапуск из настроек —\nон сам поставит юнит и права (если запускать от админа).\n")
	return b.String()
}
