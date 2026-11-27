package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"xray-tui/internal/applog"
	"xray-tui/internal/devterm"
	"xray-tui/internal/ipc"
	"xray-tui/internal/osutil"
	"xray-tui/internal/uiutil"
)

// Меню разработчика живёт внутри вкладки «Настройки» как отдельный экран
// (аналог routing-экранов). Все действия открывают внешние терминалы
// и просмотрщики; в сам TUI ничего не пишется — вкладка «Логи» удалена.

func devMenuTitles() []string {
	items := []string{
		"Логи ядра (info) — вкл/выкл",
		"Логи приложения",
		"Дамп конфига ядра",
		"Проверить туннель",
		"Статус ядра",
		"Рестарт ядра",
		"Диагностика",
	}
	if runtime.GOOS == "linux" {
		items = append(items,
			"Готовность к TUN",
			"Журнал демона (journalctl)",
		)
	}
	return items
}

func (m model) devMenuValues() []string {
	running, devLog := "—", "—"
	if st := m.devStatus; st != nil {
		if st.Running {
			running = fmt.Sprintf("PID %d", st.PID)
		} else {
			running = "не запущен"
		}
		devLog = onOffLabel(st.DevLog)
	}
	vals := []string{devLog, "открыть →", "открыть →", "запустить →", running, "перезапустить", "собрать →"}
	if runtime.GOOS == "linux" {
		tunReady := "?"
		if m.tunReadyKnown {
			tunReady = onOffLabel(m.tunReady)
		}
		vals = append(vals, tunReady, "открыть →")
	}
	return vals
}

func (m model) handleDevKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	n := len(devMenuTitles())
	switch key {
	case "esc", "q":
		m.devOpen = false
		return m, nil
	case "up", "k":
		if m.devCursor > 0 {
			m.devCursor--
		}
	case "down", "j":
		if m.devCursor < n-1 {
			m.devCursor++
		}
	case "enter", " ", "right", "l":
		return m.runDevAction(m.devCursor)
	}
	return m, nil
}

func (m model) runDevAction(idx int) (tea.Model, tea.Cmd) {
	switch idx {
	case 0: // Логи ядра: toggle info + зеркало + tail
		st := m.devStatus
		if st == nil {
			m.notice = "Статус ядра недоступен (демон не отвечает)"
			m.noticeErr = true
			return m, nil
		}
		if !st.Running {
			m.notice = "Ядро не запущено — сначала подключитесь"
			m.noticeErr = true
			return m, nil
		}
		if st.DevLog {
			if err := m.client.DevLog(false); err != nil {
				m.notice = "Ошибка: " + err.Error()
				m.noticeErr = true
				return m, nil
			}
			applog.Event("dev kernel log disabled")
			m.notice = "Отладочные логи ядра выключены"
			m.noticeErr = false
			return m, devStatusCmd(m.client)
		}
		if err := m.client.DevLog(true); err != nil {
			m.notice = "Ошибка: " + err.Error()
			m.noticeErr = true
			return m, nil
		}
		applog.Event("dev kernel log enabled")
		_ = devterm.Terminal(logTailCmd(filepath.Join(applog.Dir(), "xray.log"), 200), false)
		m.notice = "Отладочные логи ядра: info, зеркало " + filepath.Join(applog.Dir(), "xray.log")
		m.noticeErr = false
		return m, devStatusCmd(m.client)

	case 1: // Логи приложения
		_ = devterm.Terminal(logTailCmd(applog.File(), 100), false)
		return m, nil

	case 2: // Дамп конфига
		m.busy = true
		return m, devConfigDumpCmd(m.client)

	case 3: // Проверить туннель
		m.busy = true
		m.notice = "Проверяю туннель (~10 сек)..."
		m.noticeErr = false
		return m, devTunnelCheckCmd(m.client)

	case 4: // Статус ядра
		st, err := m.client.DevStatus()
		if err != nil {
			m.notice = "Демон недоступен: " + err.Error()
			m.noticeErr = true
			return m, nil
		}
		m.devStatus = st
		m.notice = devStatusLine(st)
		m.noticeErr = false
		return m, nil

	case 5: // Рестарт ядра
		return m.reconnectIfRunning("Ядро перезапускается")

	case 6: // Диагностика
		m.busy = true
		m.notice = "Собираю диагностику..."
		m.noticeErr = false
		return m, devDiagnosticsCmd(m.client)

	default:
		if runtime.GOOS != "linux" {
			return m, nil
		}
		if idx == 7 { // Готовность к TUN
			m.busy = true
			return m, devTunReadinessCmd()
		}
		if idx == 8 { // Журнал демона
			openDaemonJournal()
			return m, nil
		}
	}
	return m, nil
}

func devStatusLine(st *ipc.DevStatusResponse) string {
	if !st.Running {
		return "Ядро не запущено"
	}
	upt := time.Duration(st.UptimeSec) * time.Second
	parts := []string{
		fmt.Sprintf("PID %d", st.PID),
		"аптайм " + shortDuration(upt),
	}
	if st.Version != "" && st.Version != "unknown" {
		parts = append(parts, st.Version)
	}
	if st.Profile != "" {
		parts = append(parts, "профиль «"+st.Profile+"»")
	}
	if st.Mode != "" {
		parts = append(parts, "режим "+st.Mode)
	}
	parts = append(parts, "отладочные логи: "+onOffLabel(st.DevLog))
	return strings.Join(parts, " · ")
}

// logTailCmd — команда следящего просмотра журнала во внешнем окне:
// tail на Unix, Get-Content -Wait в PowerShell на Windows.
func logTailCmd(path string, lines int) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("Get-Content -LiteralPath '%s' -Wait -Tail %d", path, lines)
	}
	return fmt.Sprintf("tail -n %d -F %s", lines, shellQuoteArg(path))
}

func shortDuration(d time.Duration) string {
	d = d.Round(time.Second)
	min := int(d.Minutes())
	sec := int(d.Seconds()) % 60
	if min >= 60 {
		return fmt.Sprintf("%dч%02dм", min/60, min%60)
	}
	return fmt.Sprintf("%dм%02dс", min, sec)
}

func shellQuoteArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// --- Команды (tea.Cmd) ---

func devStatusCmd(client *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		st, err := client.DevStatus()
		return devStatusMsg{st: st, err: err}
	}
}

func devConfigDumpCmd(client *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.DevConfig()
		if err != nil {
			return devFileDoneMsg{title: "Дамп конфига", err: err}
		}
		path := filepath.Join(os.TempDir(), "fluffify-xray-config.json")
		if err := os.WriteFile(path, []byte(resp.Config), 0o600); err != nil {
			return devFileDoneMsg{title: "Дамп конфига", err: err}
		}
		return devFileDoneMsg{title: "Дамп конфига", path: path}
	}
}

func devTunnelCheckCmd(client *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.DevCheck()
		if err != nil {
			return devFileDoneMsg{title: "Проверка туннеля", err: err}
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Fluffify tunnel check — %s\n\n", applog.Stamp())
		fmt.Fprintf(&b, "Внешний IP через маршрут по умолчанию: %s\n", resp.TunnelIP)
		if resp.DirectIface != "" {
			fmt.Fprintf(&b, "Внешний IP мимо туннеля (%s):        %s\n", resp.DirectIface, resp.DirectIP)
		}
		fmt.Fprintf(&b, "IP системного DNS-резолвера:          %s\n", resp.ResolverIP)
		if resp.TunnelIP != "" && resp.DirectIP != "" && resp.TunnelIP == resp.DirectIP {
			b.WriteString("\nВывод: трафик идёт мимо туннеля (IP совпадают).\n")
		} else if resp.TunnelIP != "" && resp.DirectIP != "" {
			b.WriteString("\nВывод: туннель работает (IP различаются).\n")
		}
		if len(resp.Notes) > 0 {
			b.WriteString("\nЗамечания:\n")
			for _, n := range resp.Notes {
				b.WriteString("  - " + n + "\n")
			}
		}
		return writeDevReport("Проверка туннеля", "fluffify-tunnel-check.txt", b.String())
	}
}

// writeDevReport сохраняет текст отчёта во временный файл.
func writeDevReport(title, name, text string) devFileDoneMsg {
	path := filepath.Join(os.TempDir(), name)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return devFileDoneMsg{title: title, err: err}
	}
	return devFileDoneMsg{title: title, path: path}
}

// --- Диагностика ---

var (
	reURLCreds = regexp.MustCompile(`(?i)(token|key|secret|auth|password|pass|user|uuid)=([^&\s]+)`)
	reUUID     = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)
)

// maskSensitive прячет подписочные токены и UUID в текстах отчётов.
func maskSensitive(s string) string {
	s = reURLCreds.ReplaceAllString(s, "$1=masked")
	s = reUUID.ReplaceAllString(s, "<uuid>")
	return s
}

// runCmd выполняет команду и возвращает её вывод (или «недоступно»).
func runCmd(name string, args ...string) string {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Sprintf("%s: недоступно (%v)\n", name, err)
	}
	return string(out)
}

// tailFile возвращает последние n строк файла.
func tailFile(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "(" + path + ": не читается)\n"
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n") + "\n"
}

func devDiagnosticsCmd(client *ipc.Client) tea.Cmd {
	return func() tea.Msg {
		var b strings.Builder
		fmt.Fprintf(&b, "Fluffify diagnostics — %s\n", applog.Stamp())
		fmt.Fprintf(&b, "OS: %s (%s/%s)\n", osutil.OSName(), runtime.GOOS, runtime.GOARCH)

		if self, err := os.Executable(); err == nil {
			fmt.Fprintf(&b, "Executable: %s\n", self)
		}
		sock := ipc.DefaultSocketPath()
		if _, err := os.Stat(sock); err == nil {
			fmt.Fprintf(&b, "IPC socket: %s (есть)\n", sock)
		} else {
			fmt.Fprintf(&b, "IPC socket: %s (нет)\n", sock)
		}

		fmt.Fprintln(&b, "\n=== Ядро ===")
		if st, err := client.DevStatus(); err == nil {
			b.WriteString(devStatusLine(st) + "\n")
		} else {
			fmt.Fprintf(&b, "демон недоступен: %v\n", err)
		}

		if runtime.GOOS == "linux" {
			fmt.Fprintf(&b, "\n=== TUN ===\n%s", tunReadinessReport())
		}

		if runtime.GOOS == "windows" {
			fmt.Fprintf(&b, "\n=== Сеть ===\n%s", runCmd("cmd", "/c", "ipconfig", "/all"))
			fmt.Fprintf(&b, "\n=== Маршруты ===\n%s", runCmd("cmd", "/c", "route", "print"))
		} else {
			fmt.Fprintf(&b, "\n=== Интерфейсы ===\n%s", runCmd("ip", "-brief", "addr"))
			fmt.Fprintf(&b, "\n=== Маршруты ===\n%s", runCmd("ip", "route"))
			fmt.Fprintf(&b, "\n=== resolv.conf ===\n%s", tailFile("/etc/resolv.conf", 12))
		}

		fmt.Fprintln(&b, "\n=== Конфиг ядра (замаскирован) ===")
		if resp, err := client.DevConfig(); err == nil {
			b.WriteString(resp.Config + "\n")
		} else {
			fmt.Fprintf(&b, "(демон недоступен: %v)\n", err)
		}
		fmt.Fprintf(&b, "\n=== Журнал приложения (последние 50) ===\n%s", maskSensitive(tailFile(applog.File(), 50)))

		return writeDevReport("Диагностика", "fluffify-diagnostics.txt", b.String())
	}
}

// --- Готовность к TUN (Linux) ---

// tunReadyKnown/tunReady — кэш для значения в списке dev-меню.
// Обновляются по результату devTunReadinessCmd.

func devTunReadinessCmd() tea.Cmd {
	return func() tea.Msg {
		report := tunReadinessReport()
		return writeDevReport("Готовность к TUN", "fluffify-tun-readiness.txt",
			"Fluffify TUN readiness — "+applog.Stamp()+"\n\n"+report)
	}
}

// tunProbeCmd — быстрая проверка готовности к TUN для значения в списке.
func tunProbeCmd() tea.Cmd {
	return func() tea.Msg { return tunProbe() }
}

// --- Рендер ---

func (m model) renderDevMenu(w, h int) string {
	innerW := maxInt(20, w-4)
	titles := devMenuTitles()
	vals := m.devMenuValues()

	rows := make([]string, 0, len(titles)+2)
	rows = append(rows, clipLine(styleMuted.Render("  Управление логами и отладкой. Всё открывается во внешних окнах."), innerW))
	rows = append(rows, "")
	for i, title := range titles {
		prefix := "  "
		ttl := uiutil.Display(title)
		val := uiutil.Display(vals[i])
		if m.devCursor == i {
			prefix = styleAccent.Render("▸ ")
			ttl = styleCursorRow.Render(ttl)
			val = styleAccent.Render(val)
		}
		gap := innerW - termWidth(prefix) - termWidth(ttl) - termWidth(val)
		if gap < 1 {
			gap = 1
		}
		row := clipLine(prefix+ttl+strings.Repeat(" ", gap)+val, innerW)
		if m.devCursor == i {
			row = highlightRow(row, innerW)
		}
		rows = append(rows, row)
	}
	rows = append(rows, "")
	rows = append(rows, clipLine(styleMuted.Render("  esc назад"), innerW))

	body := renderBox("Меню разработчика", rows, w)
	lines := strings.Split(body, "\n")
	return padLines(lines, h)
}

// --- Мышь для dev-меню ---

// handleMouseDev обрабатывает клики по списку dev-меню (строки совпадают
// с renderDevMenu: 0:верх рамки, 1:заголовок, 2:подсказка, 3:пустая, 4+:пункты).
// Колесо обрабатывается в handleMouseWheel.
func (m model) handleMouseDev(relY, bodyH int) (tea.Model, tea.Cmd) {
	n := len(devMenuTitles())
	first := 4

	idx := relY - first
	if idx < 0 || idx >= n {
		return m, nil
	}
	m.devCursor = idx
	return m.runDevAction(idx)
}
