package tui

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"xray-tui/internal/config"
	"xray-tui/internal/parser"
)

type subFormMode int

const (
	subModeSub subFormMode = iota
	subModeLink
	subModeJSON
)

type subTabBox struct {
	x1   int
	x2   int
	mode subFormMode
}

type footerBtnID int

const (
	btnAdd footerBtnID = iota
	btnPing
	btnRefresh
	btnDelete
)

type footerBtnBox struct {
	x1      int
	x2      int
	id      footerBtnID
	enabled bool
}

func (m model) footerBtnEnabled(id footerBtnID) bool {
	switch id {
	case btnAdd:
		if m.tab == tabServers || m.tab == tabSubs {
			return true
		}
		if m.tab == tabSettings && (m.routingView == routingProfiles || m.routingView == routingProfileEdit || m.routingView == routingProviders) {
			return true
		}
		return false

	case btnPing:
		return m.tab == tabServers && len(m.store.Profiles) > 0

	case btnRefresh:
		if m.tab == tabServers || m.tab == tabSubs {
			return len(m.store.Subscriptions) > 0
		}
		if m.tab == tabSettings && m.routingView == routingProviders {
			return len(m.store.Routing.GeoProviders) > 0
		}
		return false

	case btnDelete:
		switch m.tab {
		case tabServers:
			rows := m.buildTreeRows()
			if m.cursor >= 0 && m.cursor < len(rows) {
				r := rows[m.cursor]
				if r.isHeader {
					return r.subIdx >= 0 && r.subIdx < len(m.store.Subscriptions)
				}
				return m.store.Profiles[r.profileIdx].SubID == ""
			}
			return false
		case tabSubs:
			return len(m.store.Subscriptions) > 0
		case tabSettings:
			switch m.routingView {
			case routingProfiles:
				return len(m.store.Routing.Profiles) > 0
			case routingProfileEdit:
				return m.routingEditIdx >= 0 && m.routingEditIdx < len(m.store.Routing.Profiles) &&
					len(m.store.Routing.Profiles[m.routingEditIdx].Rules) > 0
			case routingProviders:
				return len(m.store.Routing.GeoProviders) > 0
			}
		}
		return false
	}
	return false
}

func (m model) layoutFooterButtons(w int) (string, []footerBtnBox) {
	defs := []struct {
		id    footerBtnID
		glyph string
	}{
		{btnAdd, "+"},
		{btnPing, "↯"},
		{btnRefresh, "↻"},
		{btnDelete, "✕"},
	}

	const innerW = 5 // ширина каждой кнопки с рамкой = 7 столбцов, высота = 3 строки
	const boxW = innerW + 2

	activeBorder := lipgloss.NewStyle().Foreground(colorAccent)
	mutedBorder := lipgloss.NewStyle().Foreground(colorBorder)
	activeGlyph := styleAccent.Bold(true)

	var topRow, midRow, botRow strings.Builder
	topRow.WriteByte(' ')
	midRow.WriteByte(' ')
	botRow.WriteByte(' ')

	curX := 1
	boxes := make([]footerBtnBox, 0, len(defs))

	for i, d := range defs {
		if i > 0 {
			topRow.WriteByte(' ')
			midRow.WriteByte(' ')
			botRow.WriteByte(' ')
			curX++
		}

		en := m.footerBtnEnabled(d.id)
		bdr := mutedBorder
		gStyle := styleMuted
		if en {
			bdr = activeBorder
			gStyle = activeGlyph
		}

		gw := lipgloss.Width(d.glyph)
		padL := (innerW - gw) / 2
		padR := innerW - gw - padL

		topRow.WriteString(bdr.Render("╭" + strings.Repeat("─", innerW) + "╮"))
		midRow.WriteString(bdr.Render("│") + strings.Repeat(" ", padL) + gStyle.Render(d.glyph) + strings.Repeat(" ", padR) + bdr.Render("│"))
		botRow.WriteString(bdr.Render("╰" + strings.Repeat("─", innerW) + "╯"))

		boxes = append(boxes, footerBtnBox{
			x1:      curX,
			x2:      curX + boxW,
			id:      d.id,
			enabled: en,
		})
		curX += boxW
	}

	out := clipLine(topRow.String(), w) + "\n" +
		clipLine(midRow.String(), w) + "\n" +
		clipLine(botRow.String(), w)
	return out, boxes
}

func (m model) handleFooterButtonClick(w, x int, isDouble bool) (tea.Model, tea.Cmd) {
	_, boxes := m.layoutFooterButtons(w)
	for _, b := range boxes {
		if x >= b.x1 && x < b.x2 {
			if !b.enabled {
				return m, nil
			}
			switch b.id {
			case btnAdd:
				return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

			case btnPing:
				if isDouble {
					return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'P'}})
				}
				return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

			case btnRefresh:
				if m.tab == tabSubs && isDouble {
					return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'U'}})
				}
				return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})

			case btnDelete:
				if m.tab == tabServers {
					rows := m.buildTreeRows()
					if m.cursor >= 0 && m.cursor < len(rows) && rows[m.cursor].isHeader {
						return m.deleteSubscriptionByIndex(rows[m.cursor].subIdx)
					}
					return m.deleteSelectedStandaloneNode()
				}
				return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
			}
		}
	}
	return m, nil
}

func (m model) deleteSelectedStandaloneNode() (tea.Model, tea.Cmd) {
	rows := m.buildTreeRows()
	if m.cursor < 0 || m.cursor >= len(rows) || rows[m.cursor].isHeader {
		return m, nil
	}
	pIdx := rows[m.cursor].profileIdx
	p := m.store.Profiles[pIdx]
	if p.SubID != "" {
		m.notice = "Удали подписку во вкладке 2:Подп"
		m.noticeErr = true
		return m, nil
	}

	m.store.Profiles = append(m.store.Profiles[:pIdx], m.store.Profiles[pIdx+1:]...)
	if m.store.Settings.ActiveProfileID == p.ID {
		m.store.Settings.ActiveProfileID = ""
	}
	_ = m.store.Save()

	newRows := m.buildTreeRows()
	if m.cursor >= len(newRows) {
		m.cursor = maxInt(0, len(newRows)-1)
	}
	m.clampTreeScroll()
	m.notice = "Удалена нода: " + p.Name
	m.noticeErr = false
	return m, nil
}

// layoutSubFormHeader строит шапку "Новая подписка / ссылка / JSON" в стиле верхнего пути
// и возвращает кликабельные X-координаты каждой вкладки (учитывая "│ " слева = +2).
func (m model) layoutSubFormHeader(innerW int) (string, []subTabBox) {
	tabs := []struct {
		mode  subFormMode
		label string
	}{
		{subModeSub, "Новая подписка"},
		{subModeLink, "ссылка"},
		{subModeJSON, "JSON"},
	}

	activeStyle := styleAccent.Bold(true)
	sep := styleMuted.Render(" / ")

	var b strings.Builder
	boxes := make([]subTabBox, 0, len(tabs))
	curX := 2 // "│ " слева внутри рамки

	for i, t := range tabs {
		if i > 0 {
			b.WriteString(sep)
			curX += 3
		}
		lw := lipgloss.Width(t.label)
		if m.subForm.mode == t.mode {
			b.WriteString(activeStyle.Render(t.label))
		} else {
			b.WriteString(t.label)
		}
		boxes = append(boxes, subTabBox{
			x1:   curX,
			x2:   curX + lw,
			mode: t.mode,
		})
		curX += lw
	}

	return clipLine(b.String(), innerW), boxes
}

// submitAddLink — вкладка «Ключ (ссылка)»: текст разбирается умно.
func (m model) submitAddLink() (tea.Model, tea.Cmd) {
	if strings.TrimSpace(m.subForm.link) == "" {
		m.notice = "Вставь ссылку (vless://, hy2://, trojan://) или JSON"
		m.noticeErr = true
		return m, nil
	}
	return m.applySmartInput(m.subForm.link)
}

// applySmartInput — единая точка входа для вставленного текста: определяет,
// это подписка, ключ, JSON или файл, и выполняет нужное действие.
func (m model) applySmartInput(text string) (tea.Model, tea.Cmd) {
	kind, value := parser.ClassifyInput(text)
	switch kind {
	case parser.InputSubscription:
		return m.addSubscription(value)
	case parser.InputLink:
		return m.addLinks(value)
	case parser.InputJSON:
		return m.addJSONNode(value)
	case parser.InputFilePath:
		if data, err := os.ReadFile(value); err == nil {
			return m.applySmartInput(string(data))
		}
		if len(parser.TotalLines(value)) > 1 {
			return m.addLinks(value)
		}
		m.notice = "Не понял ввод: это не ссылка, не JSON и не существующий файл"
		m.noticeErr = true
		return m, nil
	default:
		m.notice = "Пустой ввод"
		m.noticeErr = true
		return m, nil
	}
}

// addSubscription создаёт подписку и запускает её загрузку.
func (m model) addSubscription(rawURL string) (tea.Model, tea.Cmd) {
	u := strings.TrimSpace(rawURL)
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		m.notice = "URL должен начинаться с http:// или https://"
		m.noticeErr = true
		return m, nil
	}

	sub := config.Subscription{
		ID:       config.NewID(),
		Name:     strings.TrimSpace(m.subForm.name),
		URL:      u,
		SendHWID: m.subForm.sendHWID,
	}
	m.store.Subscriptions = append(m.store.Subscriptions, sub)
	if err := m.store.Save(); err != nil {
		m.notice = "Ошибка сохранения: " + err.Error()
		m.noticeErr = true
		return m, nil
	}

	idx := len(m.store.Subscriptions) - 1
	m.subForm = subFormState{}
	m.busy = true
	m.notice = "Загружаю подписку..."
	m.noticeErr = false
	return m, m.syncSubsCmd(idx)
}

func (m model) addLinks(text string) (tea.Model, tea.Cmd) {
	raw := strings.TrimSpace(text)
	if raw == "" {
		m.notice = "Вставь ссылку (vless://, hy2://, trojan:// и т.д.)"
		m.noticeErr = true
		return m, nil
	}

	lines := parser.TotalLines(raw)

	added := 0
	var lastErr error
	var lastName string
	for _, line := range lines {
		line = parser.NormalizeInput(line)
		if line == "" {
			continue
		}
		node, err := parser.ParseURI(line)
		if err != nil {
			lastErr = err
			continue
		}
		name := extractURIName(line, node.Address, node.Port)
		prof := config.Profile{
			ID:       config.NewID(),
			Name:     name,
			Type:     "uri",
			Protocol: node.Protocol,
			Address:  node.Address,
			Port:     node.Port,
			URI:      line,
			SubID:    "",
			AddedAt:  time.Now(),
		}
		m.store.Profiles = append(m.store.Profiles, prof)
		added++
		lastName = name
	}

	if added == 0 {
		if lastErr != nil {
			m.notice = "Ошибка ссылки: " + lastErr.Error()
		} else {
			m.notice = "Не удалось распознать ссылку"
		}
		m.noticeErr = true
		return m, nil
	}

	if err := m.store.Save(); err != nil {
		m.notice = "Ошибка сохранения: " + err.Error()
		m.noticeErr = true
		return m, nil
	}

	m.subForm = subFormState{}
	m.tab = tabServers
	m.clampTreeScroll()
	if added == 1 {
		m.notice = "Добавлена нода: " + lastName
	} else {
		m.notice = fmt.Sprintf("Добавлено нод: %d", added)
	}
	m.noticeErr = false
	return m, nil
}

// submitAddJSON — вкладка «JSON ноды»: принимает и JSON, и ссылку, и файл.
func (m model) submitAddJSON() (tea.Model, tea.Cmd) {
	if strings.TrimSpace(m.subForm.rawJSON) == "" {
		m.notice = "Вставь JSON конфиг или outbound-ноду"
		m.noticeErr = true
		return m, nil
	}
	return m.applySmartInput(m.subForm.rawJSON)
}

func (m model) addJSONNode(text string) (tea.Model, tea.Cmd) {
	raw := []byte(strings.TrimSpace(text))
	if len(raw) == 0 {
		m.notice = "Вставь JSON конфиг или outbound-ноду"
		m.noticeErr = true
		return m, nil
	}
	if !json.Valid(raw) {
		m.notice = "Невалидный JSON"
		m.noticeErr = true
		return m, nil
	}

	prof, err := buildProfileFromRawJSON(raw)
	if err != nil {
		m.notice = err.Error()
		m.noticeErr = true
		return m, nil
	}

	m.store.Profiles = append(m.store.Profiles, prof)
	if err := m.store.Save(); err != nil {
		m.notice = "Ошибка сохранения: " + err.Error()
		m.noticeErr = true
		return m, nil
	}

	m.subForm = subFormState{}
	m.tab = tabServers
	m.clampTreeScroll()
	m.notice = "Добавлена JSON-нода: " + prof.Name
	m.noticeErr = false
	return m, nil
}

func extractURIName(rawLink, addr string, port int) string {
	if idx := strings.LastIndex(rawLink, "#"); idx >= 0 && idx+1 < len(rawLink) {
		frag := strings.TrimSpace(rawLink[idx+1:])
		if unesc, err := url.QueryUnescape(frag); err == nil && strings.TrimSpace(unesc) != "" {
			return strings.TrimSpace(unesc)
		}
		if frag != "" {
			return frag
		}
	}
	if strings.HasPrefix(strings.ToLower(rawLink), "vmess://") {
		payload := strings.TrimSpace(rawLink[len("vmess://"):])
		if dec, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(payload, "=")); err == nil {
			var obj map[string]any
			if json.Unmarshal(dec, &obj) == nil {
				if ps, _ := obj["ps"].(string); strings.TrimSpace(ps) != "" {
					return strings.TrimSpace(ps)
				}
			}
		}
	}
	if addr != "" && port > 0 {
		return fmt.Sprintf("%s:%d", addr, port)
	}
	if addr != "" {
		return addr
	}
	return "Нода"
}

func buildProfileFromRawJSON(raw []byte) (config.Profile, error) {
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return config.Profile{}, fmt.Errorf("ожидается JSON-объект")
	}

	// Если вставлен одиночный outbound без обёртки — оборачиваем в полноценный конфиг Xray
	var primary map[string]any
	if outs, ok := obj["outbounds"].([]any); ok && len(outs) > 0 {
		for _, item := range outs {
			ob, ok := item.(map[string]any)
			if !ok {
				continue
			}
			p, _ := ob["protocol"].(string)
			if p == "freedom" || p == "blackhole" || p == "dns" {
				continue
			}
			primary = ob
			break
		}
	} else if proto, _ := obj["protocol"].(string); proto != "" {
		primary = obj
		if _, hasTag := obj["tag"]; !hasTag {
			obj["tag"] = "proxy"
		}
		wrapped := map[string]any{
			"outbounds": []any{
				obj,
				map[string]any{"tag": "direct", "protocol": "freedom"},
				map[string]any{"tag": "block", "protocol": "blackhole"},
			},
		}
		b, err := json.Marshal(wrapped)
		if err != nil {
			return config.Profile{}, err
		}
		raw = b
	} else {
		return config.Profile{}, fmt.Errorf("JSON должен содержать outbounds или protocol")
	}

	proto := "raw"
	addr := ""
	port := 0
	name := ""

	for _, k := range []string{"remarks", "remark", "name"} {
		if s, _ := obj[k].(string); strings.TrimSpace(s) != "" {
			name = strings.TrimSpace(s)
			break
		}
	}

	if primary != nil {
		if p, _ := primary["protocol"].(string); p != "" {
			proto = p
		}
		if name == "" {
			if t, _ := primary["tag"].(string); t != "" && t != "proxy" {
				name = t
			}
		}
		if s, ok := primary["settings"].(map[string]any); ok {
			addr, port = extractAddrPort(s)
		}
	}

	if name == "" {
		if addr != "" {
			name = fmt.Sprintf("%s (%s)", proto, addr)
		} else {
			name = "JSON нода"
		}
	}

	return config.Profile{
		ID:       config.NewID(),
		Name:     name,
		Type:     config.ProfileTypeRaw,
		Protocol: proto,
		Address:  addr,
		Port:     port,
		RawJSON:  json.RawMessage(raw),
		SubID:    "",
		AddedAt:  time.Now(),
	}, nil
}

func extractAddrPort(settings map[string]any) (string, int) {
	if a, _ := settings["address"].(string); a != "" {
		p, _ := settings["port"].(float64)
		return a, int(p)
	}
	for _, key := range []string{"vnext", "servers"} {
		if list, ok := settings[key].([]any); ok && len(list) > 0 {
			if item, ok := list[0].(map[string]any); ok {
				a, _ := item["address"].(string)
				p, _ := item["port"].(float64)
				if a != "" {
					return a, int(p)
				}
			}
		}
	}
	return "", 0
}

func (m model) deleteSubscriptionByIndex(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.store.Subscriptions) {
		return m, nil
	}
	sub := m.store.Subscriptions[idx]
	m.store.Subscriptions = append(m.store.Subscriptions[:idx], m.store.Subscriptions[idx+1:]...)

	filtered := m.store.Profiles[:0]
	for _, p := range m.store.Profiles {
		if p.SubID == sub.ID {
			if m.store.Settings.ActiveProfileID == p.ID {
				m.store.Settings.ActiveProfileID = ""
			}
			continue
		}
		filtered = append(filtered, p)
	}
	m.store.Profiles = filtered

	if err := m.store.Save(); err != nil {
		m.notice = "Ошибка сохранения: " + err.Error()
		m.noticeErr = true
		return m, nil
	}

	if m.subCursor >= len(m.store.Subscriptions) {
		m.subCursor = maxInt(0, len(m.store.Subscriptions)-1)
	}
	newRows := m.buildTreeRows()
	if m.cursor >= len(newRows) {
		m.cursor = maxInt(0, len(newRows)-1)
	}
	m.clampTreeScroll()

	name := sub.Name
	if name == "" {
		name = sub.URL
	}
	m.notice = "Удалена подписка: " + name
	m.noticeErr = false
	return m, nil
}
