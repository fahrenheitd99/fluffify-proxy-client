package tui

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"xray-tui/internal/about"
	"xray-tui/internal/applog"
	"xray-tui/internal/autostart"
	"xray-tui/internal/config"
	"xray-tui/internal/osutil"
	"xray-tui/internal/parser"
	"xray-tui/internal/uiutil"
)

type settingsFormKind int

const (
	sFormNone settingsFormKind = iota
	sFormSocksPort
	sFormHTTPPort
	sFormAuth
	sFormPingURL
	sFormHWID
	sFormUA
)

type settingsFormState struct {
	kind        settingsFormKind
	step        int
	value       string
	valuePasted bool
	enabled     bool
	user        string
	pass        string
}

const totalSettingsItems = 22

func (m model) inSettingsForm() bool {
	return m.settingsForm.kind != sFormNone
}

func (m model) handleSettingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.inSettingsForm() {
		return m.handleSettingsFormKey(msg)
	}
	if m.devOpen {
		return m.handleDevKey(msg)
	}

	key := msg.String()
	switch key {
	case "up", "k":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case "down", "j":
		if m.settingsCursor < totalSettingsItems-1 {
			m.settingsCursor++
		}
	case "e":
		switch m.settingsCursor {
		case 7:
			m.settingsForm = settingsFormState{kind: sFormSocksPort, value: strconv.Itoa(m.store.Settings.SocksPort)}
		case 8:
			m.settingsForm = settingsFormState{kind: sFormHTTPPort, value: strconv.Itoa(m.store.Settings.GetHTTPPort())}
		case 9:
			m.settingsForm = settingsFormState{
				kind:    sFormAuth,
				enabled: m.store.Settings.ProxyAuth,
				user:    m.store.Settings.ProxyUser,
				pass:    m.store.Settings.ProxyPass,
			}
		case 12:
			m.settingsForm = settingsFormState{kind: sFormPingURL, value: m.store.Settings.PingURL}
		case 17:
			m.settingsForm = settingsFormState{kind: sFormHWID, value: m.store.Settings.HWID}
		case 18:
			m.settingsForm = settingsFormState{kind: sFormUA, value: m.store.Settings.DefaultUA}
		}
		return m, nil

	case "enter", " ", "left", "right", "h", "l":
		rev := key == "left" || key == "h"
		isAction := key == "enter" || key == " "

		switch m.settingsCursor {
		// === Настройки Xray (0..10) ===
		case 0: // Режим работы
			if m.store.Settings.Mode == config.ModeTUN {
				m.store.Settings.Mode = config.ModeProxy
			} else {
				m.store.Settings.Mode = config.ModeTUN
			}
			_ = m.store.Save()
			return m.reconnectIfRunning("Режим работы: " + modeLabel(m.store.Settings.Mode))

		case 1: // Тип пинга
			modes := []string{config.PingModeICMP, config.PingModeProxyHEAD}
			m.store.Settings.PingMode = cycleString(modes, m.store.Settings.PingMode, rev)
			_ = m.store.Save()
			m.notice = "Тип пинга: " + pingModeLabel(m.store.Settings.PingMode)
			m.noticeErr = false

		case 2: // Правила роутинга
			if isAction || key == "right" || key == "l" {
				m.routingView = routingProfiles
				m.routingCursor = 0
			}

		case 3: // Перехват DNS в TUN
			m.store.Settings.SetTUNAutoDNS(!m.store.Settings.TUNAutoDNSValue())
			_ = m.store.Save()
			return m.reconnectIfRunning("DNS в TUN: " + onOffLabel(m.store.Settings.TUNAutoDNSValue()))

		case 4: // IPv6 в TUN
			m.store.Settings.SetTUNIPv6(!m.store.Settings.TUNIPv6Value())
			_ = m.store.Save()
			return m.reconnectIfRunning("IPv6 в TUN: " + onOffLabel(m.store.Settings.TUNIPv6Value()))

		case 5: // Sniffing
			m.store.Settings.SetSniffing(!m.store.Settings.SniffingEnabled())
			_ = m.store.Save()
			return m.reconnectIfRunning("Sniffing: " + onOffLabel(m.store.Settings.SniffingEnabled()))

		case 6: // Mixed inbound
			m.store.Settings.MixedInbound = !m.store.Settings.MixedInbound
			_ = m.store.Save()
			return m.reconnectIfRunning("Mixed inbound: " + onOffLabel(m.store.Settings.MixedInbound))

		case 7: // Порт SOCKS5
			if isAction {
				m.settingsForm = settingsFormState{kind: sFormSocksPort, value: strconv.Itoa(m.store.Settings.SocksPort)}
			}

		case 8: // Порт HTTP
			if isAction {
				m.settingsForm = settingsFormState{kind: sFormHTTPPort, value: strconv.Itoa(m.store.Settings.GetHTTPPort())}
			}

		case 9: // Авторизация прокси
			if isAction {
				m.settingsForm = settingsFormState{
					kind:    sFormAuth,
					enabled: m.store.Settings.ProxyAuth,
					user:    m.store.Settings.ProxyUser,
					pass:    m.store.Settings.ProxyPass,
				}
			}

		case 10: // Уровень логов
			levels := []string{"error", "warning", "info"}
			m.store.Settings.LogLevel = cycleString(levels, m.store.Settings.LogLevel, rev)
			_ = m.store.Save()
			return m.reconnectIfRunning("Уровень логов: " + m.store.Settings.LogLevel)

		// === Настройки приложения (11..18) ===
		case 11: // Сортировка по пингу
			m.store.Settings.AutoSortPing = !m.store.Settings.AutoSortPing
			_ = m.store.Save()
			m.notice = "Сортировка по пингу: " + onOffLabel(m.store.Settings.AutoSortPing)
			m.noticeErr = false

		case 12: // URL для via proxy HEAD
			m.store.Settings.PingURL = cycleString(config.PresetPingURLs, m.store.Settings.PingURL, rev)
			_ = m.store.Save()
			m.notice = "URL пинга: " + shortPingURL(m.store.Settings.PingURL)
			m.noticeErr = false

		case 13: // Автозапуск при старте системы
			target := !m.autostartEnabled
			if err := autostart.Set(target); err != nil {
				m.notice = "Ошибка автозапуска: " + err.Error()
				m.noticeErr = true
			} else {
				m.autostartEnabled = target
				m.notice = "Автозапуск системы: " + onOffLabel(target)
				m.noticeErr = false
			}

		case 14: // Автоподключение при старте демона
			m.store.Settings.AutoConnect = !m.store.Settings.AutoConnect
			_ = m.store.Save()
			m.notice = "Автоподключение: " + onOffLabel(m.store.Settings.AutoConnect)
			m.noticeErr = false

		case 15: // Тема оформления
			m.store.Settings.Theme = cycleString(themeOrder, m.store.Settings.Theme, rev)
			applyTheme(m.store.Settings.Theme, m.store.Settings.AccentColor)
			_ = m.store.Save()
			m.notice = ""
			m.noticeErr = false

		case 16: // Акцентный цвет
			m.store.Settings.AccentColor = cycleString(accentOrder, m.store.Settings.AccentColor, rev)
			applyTheme(m.store.Settings.Theme, m.store.Settings.AccentColor)
			_ = m.store.Save()
			m.notice = ""
			m.noticeErr = false

		case 17: // HWID
			if isAction {
				m.settingsForm = settingsFormState{kind: sFormHWID, value: m.store.Settings.HWID}
			}

		case 18: // User-Agent
			if isAction {
				m.settingsForm = settingsFormState{kind: sFormUA, value: m.store.Settings.DefaultUA}
			}

		case 19: // Интерфейс (Мышь / клавиши <-> Мышь / кнопки)
			if m.store.Settings.UIMode == config.UIModeButtons {
				m.store.Settings.UIMode = config.UIModeKeys
			} else {
				m.store.Settings.UIMode = config.UIModeButtons
			}
			_ = m.store.Save()
			m.notice = "Интерфейс: " + uiModeLabel(m.store.Settings.UIMode)
			m.noticeErr = false

		case 20: // О приложении / лицензии — открываем текст лицензий из системы
			if isAction {
				path, err := about.OpenLicenseFile()
				if err != nil {
					m.notice = "Не удалось открыть лицензии: " + err.Error()
					m.noticeErr = true
					return m, nil
				}
				m.notice = "Лицензии сохранены в " + path + " и открыты"
				m.noticeErr = false
				return m, nil
			}

		case 21: // Меню разработчика
			if isAction || key == "right" || key == "l" {
				m.devOpen = true
				m.devCursor = 0
				applog.Event("dev menu opened")
				return m, tea.Batch(devStatusCmd(m.client), tunProbeCmd())
			}
		}
	}

	return m, nil
}

func (m model) reconnectIfRunning(msg string) (tea.Model, tea.Cmd) {
	if m.status != nil && m.status.Running {
		if active := m.store.ActiveProfile(); active != nil {
			m.busy = true
			m.notice = msg + " (переподключаю...)"
			m.noticeErr = false
			return m, m.connectProfileCmd(*active)
		}
	}
	m.notice = msg
	m.noticeErr = false
	return m, nil
}

func (m model) handleSettingsFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "esc" {
		m.settingsForm = settingsFormState{}
		return m, nil
	}

	switch m.settingsForm.kind {
	case sFormSocksPort, sFormHTTPPort:
		switch key {
		case "enter":
			p, err := strconv.Atoi(strings.TrimSpace(m.settingsForm.value))
			minPort := 1
			if m.settingsForm.kind == sFormHTTPPort {
				minPort = 0
			}
			if err != nil || p < minPort || p > 65535 {
				m.notice = fmt.Sprintf("Порт должен быть от %d до 65535", minPort)
				m.noticeErr = true
				return m, nil
			}
			if m.settingsForm.kind == sFormSocksPort {
				m.store.Settings.SocksPort = p
			} else {
				m.store.Settings.SetHTTPPort(p)
			}
			_ = m.store.Save()
			m.settingsForm = settingsFormState{}
			return m.reconnectIfRunning(fmt.Sprintf("Порт сохранён: %d", p))
		case "backspace":
			m.settingsForm.value = trimLastRune(m.settingsForm.value)
		default:
			if msg.Type == tea.KeyRunes {
				for _, r := range msg.Runes {
					if r >= '0' && r <= '9' && len(m.settingsForm.value) < 5 {
						m.settingsForm.value += string(r)
					}
				}
			}
		}

	case sFormPingURL:
		switch key {
		case "enter":
			u := parser.NormalizeInput(m.settingsForm.value)
			if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
				m.notice = "URL должен начинаться с http:// или https://"
				m.noticeErr = true
				return m, nil
			}
			m.store.Settings.PingURL = u
			_ = m.store.Save()
			m.settingsForm = settingsFormState{}
			m.notice = "URL пинга сохранён"
			m.noticeErr = false
		case "backspace":
			m.settingsForm.value = trimLastRune(m.settingsForm.value)
		default:
			if add := extractInputText(msg); add != "" {
				m.settingsForm.value += add
			}
		}

	case sFormHWID:
		switch key {
		case "enter":
			v := strings.TrimSpace(m.settingsForm.value)
			if v == "" {
				v = config.GenerateHWID()
			}
			m.store.Settings.HWID = v
			_ = m.store.Save()
			m.settingsForm = settingsFormState{}
			m.notice = "HWID сохранён"
			m.noticeErr = false
		case "ctrl+r":
			m.settingsForm.value = config.RandomHWID()
		case "backspace":
			m.settingsForm.value = trimLastRune(m.settingsForm.value)
		default:
			if add := extractInputText(msg); add != "" {
				m.settingsForm.value += add
			}
		}

	case sFormUA:
		switch key {
		case "enter":
			v := strings.TrimSpace(m.settingsForm.value)
			if v == "" {
				v = "Fluffify/1.0.0"
			}
			m.store.Settings.DefaultUA = v
			_ = m.store.Save()
			m.settingsForm = settingsFormState{}
			m.notice = "User-Agent сохранён"
			m.noticeErr = false
		case "backspace":
			m.settingsForm.value = trimLastRune(m.settingsForm.value)
		default:
			if add := extractInputText(msg); add != "" {
				m.settingsForm.value += add
			}
		}

	case sFormAuth:
		switch key {
		case "enter":
			if m.settingsForm.step == 0 && !m.settingsForm.enabled {
				m.store.Settings.ProxyAuth = false
				_ = m.store.Save()
				m.settingsForm = settingsFormState{}
				return m.reconnectIfRunning("Авторизация прокси: Выкл")
			}
			if m.settingsForm.step < 2 {
				m.settingsForm.step++
				return m, nil
			}
			user := strings.TrimSpace(m.settingsForm.user)
			pass := strings.TrimSpace(m.settingsForm.pass)
			if m.settingsForm.enabled && (user == "" || pass == "") {
				m.notice = "Укажи и логин, и пароль"
				m.noticeErr = true
				return m, nil
			}
			m.store.Settings.ProxyAuth = m.settingsForm.enabled
			m.store.Settings.ProxyUser = user
			m.store.Settings.ProxyPass = pass
			_ = m.store.Save()
			m.settingsForm = settingsFormState{}
			return m.reconnectIfRunning("Авторизация прокси сохранена")
		case "tab", "right", "left", " ":
			if m.settingsForm.step == 0 {
				m.settingsForm.enabled = !m.settingsForm.enabled
			}
		case "up":
			if m.settingsForm.step > 0 {
				m.settingsForm.step--
			}
		case "down":
			if m.settingsForm.step < 2 {
				m.settingsForm.step++
			}
		case "backspace":
			if m.settingsForm.step == 1 {
				m.settingsForm.user = trimLastRune(m.settingsForm.user)
			} else if m.settingsForm.step == 2 {
				m.settingsForm.pass = trimLastRune(m.settingsForm.pass)
			}
		default:
			if m.settingsForm.step == 1 || m.settingsForm.step == 2 {
				if add := extractInputText(msg); add != "" {
					if m.settingsForm.step == 1 {
						m.settingsForm.user += add
					} else {
						m.settingsForm.pass += add
					}
				}
			}
		}
	}

	return m, nil
}

func (m model) renderSettings(w, h int) string {
	if m.devOpen {
		return m.renderDevMenu(w, h)
	}
	if m.inSettingsForm() {
		return m.renderSettingsForm(w, h)
	}

	innerW := maxInt(20, w-4)

	routingVal := "Выкл →"
	if m.store.Routing.Enabled {
		if rp := m.store.ActiveRoutingProfile(); rp != nil {
			routingVal = clipLine(uiutil.Display(rp.Name), 14) + " →"
		} else {
			routingVal = "Вкл →"
		}
	}

	httpPortVal := strconv.Itoa(m.store.Settings.GetHTTPPort())
	if m.store.Settings.MixedInbound {
		httpPortVal = "в Mixed"
	} else if m.store.Settings.GetHTTPPort() == 0 {
		httpPortVal = "Выкл"
	}

	authVal := "Выкл"
	if m.store.Settings.ProxyAuth && m.store.Settings.ProxyUser != "" {
		authVal = clipLine(m.store.Settings.ProxyUser, 10) + ":***"
	}

	// UA и HWID в списке не показываем в открытом виде — только блюр
	// шрифтом Брайля. Текущее значение видно лишь в форме редактирования.
	const blurred = "⠿⠿⠿⠿⠿⠿⠿⠿⠿⠿"
	hwidVal := ""
	if strings.TrimSpace(m.store.Settings.HWID) != "" {
		hwidVal = blurred
	}
	uaVal := ""
	if strings.TrimSpace(m.store.Settings.DefaultUA) != "" {
		uaVal = blurred
	}

	xrayRows := []string{
		renderSettingRow("Режим работы", modeLabel(m.store.Settings.Mode), m.settingsCursor == 0, innerW),
		renderSettingRow("Тип пинга", pingModeLabel(m.store.Settings.PingMode), m.settingsCursor == 1, innerW),
		renderSettingRow("Правила роутинга", routingVal, m.settingsCursor == 2, innerW),
		renderSettingRow("DNS в TUN", onOffLabel(m.store.Settings.TUNAutoDNSValue()), m.settingsCursor == 3, innerW),
		renderSettingRow("IPv6 в TUN (::/0)", onOffLabel(m.store.Settings.TUNIPv6Value()), m.settingsCursor == 4, innerW),
		renderSettingRow("Sniffing", onOffLabel(m.store.Settings.SniffingEnabled()), m.settingsCursor == 5, innerW),
		renderSettingRow("Mixed (SOCKS+HTTP)", onOffLabel(m.store.Settings.MixedInbound), m.settingsCursor == 6, innerW),
		renderSettingRow("Порт SOCKS5", strconv.Itoa(m.store.Settings.SocksPort), m.settingsCursor == 7, innerW),
		renderSettingRow("Порт HTTP", httpPortVal, m.settingsCursor == 8, innerW),
		renderSettingRow("Авторизация прокси", authVal, m.settingsCursor == 9, innerW),
		renderSettingRow("Уровень логов", m.store.Settings.LogLevel, m.settingsCursor == 10, innerW),
	}

	appRows := []string{
		renderSettingRow("Сортировка по пингу", onOffLabel(m.store.Settings.AutoSortPing), m.settingsCursor == 11, innerW),
		renderSettingRow("URL пинга (HEAD)", shortPingURL(m.store.Settings.PingURL), m.settingsCursor == 12, innerW),
		renderSettingRow("Автозапуск системы", onOffLabel(m.autostartEnabled), m.settingsCursor == 13, innerW),
		renderSettingRow("Автоподключение", onOffLabel(m.store.Settings.AutoConnect), m.settingsCursor == 14, innerW),
		renderSettingRow("Тема оформления", themeLabel(m.store.Settings.Theme), m.settingsCursor == 15, innerW),
		renderSettingRow("Акцентный цвет", accentLabel(m.store.Settings.AccentColor), m.settingsCursor == 16, innerW),
		renderSettingRow("HWID", hwidVal, m.settingsCursor == 17, innerW),
		renderSettingRow("User-Agent", uaVal, m.settingsCursor == 18, innerW),
		renderSettingRow("Интерфейс", uiModeLabel(m.store.Settings.UIMode), m.settingsCursor == 19, innerW),
		renderSettingRow("О приложении / лицензии", "открыть →", m.settingsCursor == 20, innerW),
		renderSettingRow("Меню разработчика", "открыть →", m.settingsCursor == 21, innerW),
	}

	xrayLines := strings.Split(renderBox("Настройки Xray", xrayRows, w), "\n")
	appLines := strings.Split(renderBox("Настройки приложения", appRows, w), "\n")

	allLines := make([]string, 0, len(xrayLines)+len(appLines))
	allLines = append(allLines, xrayLines...)
	allLines = append(allLines, appLines...)

	var selectedLine int
	if m.settingsCursor <= 10 {
		selectedLine = 2 + m.settingsCursor
	} else {
		selectedLine = len(xrayLines) + 2 + (m.settingsCursor - 11)
	}

	start := 0
	if len(allLines) > h {
		if selectedLine >= h-2 {
			start = selectedLine - h + 3
		}
		if start+h > len(allLines) {
			start = len(allLines) - h
		}
		if start < 0 {
			start = 0
		}
	}
	end := start + h
	if end > len(allLines) {
		end = len(allLines)
	}

	return padLines(allLines[start:end], h)
}

func (m model) renderSettingsForm(w, h int) string {
	innerW := maxInt(20, w-4)
	bar := styleAccent.Render("█")

	var title string
	var rows []string

	switch m.settingsForm.kind {
	case sFormSocksPort:
		title = "Порт SOCKS5 (и Mixed)"
		rows = append(rows, wrapField("Порт (1-65535)", m.settingsForm.value+bar, true, innerW)...)
	case sFormHTTPPort:
		title = "Порт HTTP прокси"
		rows = append(rows, wrapField("Порт (0=выкл)", m.settingsForm.value+bar, true, innerW)...)
	case sFormPingURL:
		title = "URL для via proxy HEAD"
		rows = append(rows, wrapField("URL", fieldValue(m.settingsForm.value, m.settingsForm.valuePasted)+bar, true, innerW)...)
	case sFormHWID:
		title = "Кастомный HWID"
		rows = append(rows, wrapField("HWID", fieldValue(m.settingsForm.value, m.settingsForm.valuePasted)+bar, true, innerW)...)
		rows = append(rows, clipLine(styleMuted.Render("  Ctrl+R — случайный · пусто = системный"), innerW))
	case sFormUA:
		title = "User-Agent по умолчанию"
		rows = append(rows, wrapField("User-Agent", fieldValue(m.settingsForm.value, m.settingsForm.valuePasted)+bar, true, innerW)...)
		rows = append(rows, clipLine(styleMuted.Render("  пусто = Fluffify/1.0.0"), innerW))
	case sFormAuth:
		title = "Авторизация SOCKS / HTTP"
		uVal, pVal := m.settingsForm.user, m.settingsForm.pass
		if m.settingsForm.step == 1 {
			uVal += bar
		}
		if m.settingsForm.step == 2 {
			pVal += bar
		}
		rows = append(rows,
			renderFormField("1. Статус (←/→)", "["+onOffLabel(m.settingsForm.enabled)+"]", m.settingsForm.step == 0, innerW),
		)
		rows = append(rows, wrapField("2. Логин", uVal, m.settingsForm.step == 1, innerW)...)
		rows = append(rows, wrapField("3. Пароль", pVal, m.settingsForm.step == 2, innerW)...)
	}

	box := renderBox(title, rows, w)
	lines := append([]string{""}, strings.Split(box, "\n")...)
	lines = append(lines, "", clipLine(styleMuted.Render("  Enter — далее / сохранить · Esc — отмена"), w))
	return padLines(lines, h)
}

func onOffLabel(v bool) string {
	if v {
		return "Вкл"
	}
	return "Выкл"
}

func shortPingURL(raw string) string {
	switch {
	case strings.Contains(raw, "cloudflare.com"):
		return "Cloudflare"
	case strings.Contains(raw, "gstatic.com"):
		return "gstatic"
	case strings.Contains(raw, "google.com"):
		return "Google"
	default:
		s := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
		return clipLine(s, 14)
	}
}

func daemonStartHint() string {
	if runtime.GOOS == "windows" {
		return osutil.SelfName() + " restart-admin — демон с правами админа (нужно для TUN)"
	}
	if autostart.Enabled() {
		return "systemctl --user start xray-tui"
	}
	return "xray-tui daemon"
}

// wrapField переносит длинный текст на несколько строк внутри формы, чтобы всё было видно.
func wrapField(label, value string, active bool, innerW int) []string {
	prefix := "  "
	lbl := styleMuted.Render(label + ":")
	if active {
		prefix = styleAccent.Render("▸ ")
		lbl = styleTitle.Render(label + ":")
	}
	rows := []string{clipLine(prefix+lbl, innerW)}
	if value == "" {
		return rows
	}

	contentW := maxInt(8, innerW-3)
	for _, line := range wrapANSI(value, contentW) {
		rows = append(rows, clipLine("   "+line, innerW))
	}
	return rows
}

func wrapANSI(s string, maxW int) []string {
	if maxW <= 0 || s == "" {
		return []string{""}
	}

	var out []string
	var cur strings.Builder
	var activeSGR strings.Builder
	curW := 0

	for i := 0; i < len(s); {
		// Не разрываем ANSI CSI-последовательности (\x1b[...m) и не учитываем их в ширине
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7E) {
				j++
			}
			if j < len(s) {
				j++
			}
			seq := s[i:j]
			cur.WriteString(seq)
			if seq == "\x1b[0m" || seq == "\x1b[m" {
				activeSGR.Reset()
			} else if strings.HasSuffix(seq, "m") {
				activeSGR.WriteString(seq)
			}
			i = j
			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		i += size

		if r == '\n' {
			if activeSGR.Len() > 0 {
				cur.WriteString("\x1b[0m")
			}
			out = append(out, cur.String())
			cur.Reset()
			if activeSGR.Len() > 0 {
				cur.WriteString(activeSGR.String())
			}
			curW = 0
			continue
		}

		rw := lipgloss.Width(string(r))
		if curW > 0 && curW+rw > maxW {
			if activeSGR.Len() > 0 {
				cur.WriteString("\x1b[0m")
			}
			out = append(out, cur.String())
			cur.Reset()
			if activeSGR.Len() > 0 {
				cur.WriteString(activeSGR.String())
			}
			curW = 0
		}

		cur.WriteRune(r)
		curW += rw
	}

	if cur.Len() > 0 || len(out) == 0 {
		if activeSGR.Len() > 0 {
			cur.WriteString("\x1b[0m")
		}
		out = append(out, cur.String())
	}
	return out
}

func uiModeLabel(mode string) string {
	if mode == config.UIModeButtons {
		return "Мышь / кнопки"
	}
	return "Мышь / клавиши"
}
