package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"xray-tui/internal/config"
)

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	w := m.width
	if w < 38 {
		w = 38
	}

	// Колёсико мыши (в формах не скроллим)
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if m.subForm.active || m.inSettingsForm() || m.inRoutingForm() {
			return m, nil
		}
		return m.handleMouseWheel(-1)
	case tea.MouseButtonWheelDown:
		if m.subForm.active || m.inSettingsForm() || m.inRoutingForm() {
			return m, nil
		}
		return m.handleMouseWheel(1)
	}

	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft && msg.Button != tea.MouseButtonRight {
		return m, nil
	}

	now := time.Now()
	isDouble := msg.Button == tea.MouseButtonLeft &&
		now.Sub(m.lastClickAt) <= 400*time.Millisecond &&
		m.lastClickY == msg.Y &&
		absInt(m.lastClickX-msg.X) <= 2

	if msg.Button == tea.MouseButtonLeft {
		m.lastClickAt = now
		m.lastClickX = msg.X
		m.lastClickY = msg.Y
	}

	pathLines, pathBoxes := m.layoutBreadcrumbs(w)
	pathH := len(pathLines)

	// 0. Клик по пути сверху — работает даже из открытых форм и поиска
	if msg.Button == tea.MouseButtonLeft && msg.Y >= 0 && msg.Y < pathH {
		crumbs := m.breadcrumbs()
		for _, box := range pathBoxes {
			if msg.Y == box.y && msg.X >= box.x1 && msg.X < box.x2 {
				if box.idx >= 0 && box.idx < len(crumbs) && crumbs[box.idx].action != nil {
					crumbs[box.idx].action(&m)
				}
				return m, nil
			}
		}
		return m, nil
	}

	headerY := pathH
	headerH := strings.Count(m.renderHeader(w), "\n") + 1
	tabsY := headerY + headerH
	bodyY := tabsY + 1
	if m.tab == tabServers && (m.searching || m.filter != "") {
		bodyY++
	}
	bodyH := m.listHeight()

	// 1. Клик по кнопке ON / OFF в шапке
	if msg.Button == tea.MouseButtonLeft && msg.Y >= headerY+1 && msg.Y < headerY+headerH-1 {
		if msg.X >= w-statusButtonW-3 && msg.X < w-1 {
			if !m.busy {
				return m.toggleConnection()
			}
			return m, nil
		}
	}

	// 2. Клик по вкладкам — закрывает формы и переключает вкладку
	if msg.Button == tea.MouseButtonLeft && msg.Y == tabsY {
		if idx := m.tabIndexAt(w, msg.X); idx >= 0 {
			m.closeAllForms()
			if tabID(idx) == tabSettings && m.tab == tabSettings && m.routingView != routingNone {
				m.routingView = routingNone
			} else {
				m.tab = tabID(idx)
			}
			return m, nil
		}
	}

	// 2.5. Клики по нижней панели кнопок (3 строки рамки в режиме «Мышь / кнопки»)
	footerBtnY := bodyY + bodyH + 1
	if m.notice != "" {
		footerBtnY++
	}
	if msg.Button == tea.MouseButtonLeft && m.store.Settings.UIMode == config.UIModeButtons && msg.Y >= footerBtnY && msg.Y < footerBtnY+3 {
		if !m.subForm.active && !m.inSettingsForm() && !m.inRoutingForm() {
			return m.handleFooterButtonClick(w, msg.X, isDouble)
		}
		return m, nil
	}

	// Клик по вкладкам внутри окна добавления (Новая подписка / ссылка / JSON)
	if m.subForm.active {
		if msg.Button == tea.MouseButtonLeft && msg.Y == bodyY+2 {
			_, boxes := m.layoutSubFormHeader(maxInt(20, w-4))
			for _, b := range boxes {
				if msg.X >= b.x1 && msg.X < b.x2 {
					m.subForm.mode = b.mode
					m.subForm.step = 0
					return m, nil
				}
			}
		}
		return m, nil
	}

	// Внутри остальных открытых форм клики по телу игнорируем
	if m.inSettingsForm() || m.inRoutingForm() {
		return m, nil
	}

	// 3. Клик внутри основной области
	if msg.Y < bodyY || msg.Y >= bodyY+bodyH {
		return m, nil
	}
	relY := msg.Y - bodyY

	switch m.tab {
	case tabServers:
		return m.handleMouseServers(relY, msg.Button, isDouble)
	case tabSubs:
		return m.handleMouseSubs(relY, bodyH, msg.Button, isDouble)
	case tabSettings:
		if m.routingView != routingNone {
			return m.handleMouseRouting(msg.X, relY, bodyH, msg.Button, isDouble)
		}
		if m.devOpen && !m.inSettingsForm() {
			return m.handleMouseDev(relY, bodyH)
		}
		return m.handleMouseSettings(w, relY, bodyH, msg.Button, isDouble)
	}

	return m, nil
}

func (m model) handleMouseWheel(dir int) (tea.Model, tea.Cmd) {
	// Меню разработчика: колесо листает список.
	if m.tab == tabSettings && m.devOpen && !m.inSettingsForm() {
		n := len(devMenuTitles())
		m.devCursor = clampInt(m.devCursor+dir*3, 0, n-1)
		return m, nil
	}
	switch m.tab {
	case tabServers:
		rows := m.buildTreeRows()
		if len(rows) == 0 {
			return m, nil
		}
		m.cursor += dir * 3
		if m.cursor < 0 {
			m.cursor = 0
		}
		if m.cursor >= len(rows) {
			m.cursor = len(rows) - 1
		}
		m.clampTreeScroll()

	case tabSubs:
		n := len(m.store.Subscriptions)
		if n == 0 {
			return m, nil
		}
		m.subCursor += dir
		if m.subCursor < 0 {
			m.subCursor = 0
		}
		if m.subCursor >= n {
			m.subCursor = n - 1
		}

	case tabSettings:
		switch m.routingView {
		case routingNone:
			m.settingsCursor += dir
			if m.settingsCursor < 0 {
				m.settingsCursor = 0
			}
			if m.settingsCursor >= totalSettingsItems {
				m.settingsCursor = totalSettingsItems - 1
			}
		case routingProfiles:
			total := 2 + len(m.store.Routing.Profiles)
			m.routingCursor += dir
			if m.routingCursor < 0 {
				m.routingCursor = 0
			}
			if m.routingCursor >= total {
				m.routingCursor = total - 1
			}
		case routingProfileEdit:
			if m.routingEditIdx >= 0 && m.routingEditIdx < len(m.store.Routing.Profiles) {
				total := 5 + len(m.store.Routing.Profiles[m.routingEditIdx].Rules)
				m.routingRuleCursor += dir
				if m.routingRuleCursor < 0 {
					m.routingRuleCursor = 0
				}
				if m.routingRuleCursor >= total {
					m.routingRuleCursor = total - 1
				}
			}
		case routingProviders:
			n := len(m.store.Routing.GeoProviders)
			if n > 0 {
				m.routingProvCursor += dir
				if m.routingProvCursor < 0 {
					m.routingProvCursor = 0
				}
				if m.routingProvCursor >= n {
					m.routingProvCursor = n - 1
				}
			}
		}
	}

	return m, nil
}

func (m model) handleMouseServers(relY int, btn tea.MouseButton, isDouble bool) (tea.Model, tea.Cmd) {
	rows := m.buildTreeRows()
	idx := m.offset + relY
	if idx < 0 || idx >= len(rows) {
		return m, nil
	}

	m.searching = false
	m.cursor = idx
	m.clampTreeScroll()

	r := rows[idx]
	if r.isHeader {
		if isDouble && btn == tea.MouseButtonLeft && m.filter == "" {
			m.collapsed[r.groupID] = !m.collapsed[r.groupID]
			m.clampTreeScroll()
		}
		return m, nil
	}

	if isDouble {
		return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	}
	return m, nil
}

func (m model) handleMouseSubs(relY, bodyH int, btn tea.MouseButton, isDouble bool) (tea.Model, tea.Cmd) {
	if len(m.store.Subscriptions) == 0 {
		return m, nil
	}

	type span struct{ start, end int }
	spans := make([]span, len(m.store.Subscriptions))
	total := 0
	cursorStart := 0

	for i, sub := range m.store.Subscriptions {
		if i == m.subCursor {
			cursorStart = total
		}
		h := 6
		if sub.Info != nil {
			h = 7
		}
		spans[i] = span{start: total, end: total + h}
		total += h
	}

	scrollStart := 0
	if cursorStart+6 > bodyH {
		scrollStart = cursorStart - 2
	}
	if scrollStart < 0 {
		scrollStart = 0
	}

	lineIdx := scrollStart + relY
	for i, sp := range spans {
		if lineIdx >= sp.start && lineIdx < sp.end {
			m.subCursor = i
			if isDouble && btn == tea.MouseButtonLeft {
				return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			}
			break
		}
	}
	return m, nil
}

func (m model) handleMouseSettings(_, relY, bodyH int, btn tea.MouseButton, isDouble bool) (tea.Model, tea.Cmd) {
	const (
		xrayLinesCount = 14 // 0:верх, 1:заголовок, 2..12:пункты 0..10, 13:низ
		totalLines     = 28 // 14:верх, 15:заголовок, 16..26:пункты 11..21, 27:низ
	)

	var selectedLine int
	if m.settingsCursor <= 10 {
		selectedLine = 2 + m.settingsCursor
	} else {
		selectedLine = xrayLinesCount + 2 + (m.settingsCursor - 11)
	}

	start := 0
	if totalLines > bodyH {
		if selectedLine >= bodyH-2 {
			start = selectedLine - bodyH + 3
		}
		if start+bodyH > totalLines {
			start = totalLines - bodyH
		}
		if start < 0 {
			start = 0
		}
	}

	absY := start + relY
	target := -1
	switch {
	case absY >= 2 && absY <= 12:
		target = absY - 2
	case absY >= 16 && absY <= 26:
		target = 11 + (absY - 16)
	default:
		return m, nil
	}

	wasSelected := target == m.settingsCursor
	m.settingsCursor = target

	if btn == tea.MouseButtonRight {
		switch target {
		case 7, 8, 9, 12, 17, 18:
			return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
		default:
			return m.Update(tea.KeyMsg{Type: tea.KeyLeft})
		}
	}

	if wasSelected || isDouble {
		return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	}
	return m, nil
}

func (m model) handleMouseRouting(x, relY, bodyH int, btn tea.MouseButton, isDouble bool) (tea.Model, tea.Cmd) {
	switch m.routingView {
	case routingProfiles:
		target := -1
		switch {
		case relY == 2:
			target = 0
		case relY == 3:
			target = 1
		case relY >= 6:
			idx := relY - 6
			if idx < len(m.store.Routing.Profiles) {
				target = 2 + idx
			}
		}
		if target < 0 {
			return m, nil
		}
		wasSelected := m.routingCursor == target
		m.routingCursor = target
		if btn == tea.MouseButtonRight && target >= 2 {
			return m.Update(tea.KeyMsg{Type: tea.KeySpace})
		}
		if wasSelected || isDouble {
			return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		}

	case routingProfileEdit:
		if m.routingEditIdx < 0 || m.routingEditIdx >= len(m.store.Routing.Profiles) {
			return m, nil
		}
		rp := m.store.Routing.Profiles[m.routingEditIdx]

		if relY >= 2 && relY <= 6 {
			target := relY - 2
			wasSelected := m.routingRuleCursor == target
			m.routingRuleCursor = target
			if btn == tea.MouseButtonRight {
				return m.Update(tea.KeyMsg{Type: tea.KeyLeft})
			}
			if wasSelected || isDouble {
				return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			}
			return m, nil
		}

		if relY >= 9 && len(rp.Rules) > 0 {
			avail := maxInt(2, bodyH-9)
			start := 0
			if sel := m.routingRuleCursor - 5; sel >= avail {
				start = sel - avail + 1
			}
			ruleIdx := start + (relY - 9)
			if ruleIdx >= 0 && ruleIdx < len(rp.Rules) {
				target := 5 + ruleIdx
				wasSelected := m.routingRuleCursor == target
				m.routingRuleCursor = target

				if btn == tea.MouseButtonRight || (x >= 2 && x <= 5) {
					return m.Update(tea.KeyMsg{Type: tea.KeySpace})
				}
				if wasSelected || isDouble {
					return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				}
			}
		}

	case routingProviders:
		if relY >= 1 {
			idx := (relY - 1) / 4
			if idx >= 0 && idx < len(m.store.Routing.GeoProviders) {
				wasSelected := m.routingProvCursor == idx
				m.routingProvCursor = idx
				if wasSelected || isDouble {
					return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				}
			}
		}
	}

	return m, nil
}

func (m model) tabIndexAt(w, x int) int {
	var labels [3]string
	if w < 58 {
		labels = [3]string{
			fmt.Sprintf("1:Серв(%d)", len(m.store.Profiles)),
			fmt.Sprintf("2:Подп(%d)", len(m.store.Subscriptions)),
			"3:Настр",
		}
	} else {
		labels = [3]string{
			fmt.Sprintf("[1] Сервера (%d)", len(m.store.Profiles)),
			fmt.Sprintf("[2] Подписки (%d)", len(m.store.Subscriptions)),
			"[3] Настройки",
		}
	}

	sepW := 2
	if w < 52 {
		sepW = 1
	}
	curX := 1
	for i, lbl := range labels {
		lw := lipgloss.Width(lbl)
		if x >= curX && x < curX+lw {
			return i
		}
		curX += lw + sepW
	}
	return -1
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
