package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type breadcrumb struct {
	label  string
	action func(*model)
}

type crumbBox struct {
	y   int
	x1  int
	x2  int
	idx int
}

func (m *model) closeAllForms() {
	m.searching = false
	m.subForm = subFormState{}
	m.settingsForm = settingsFormState{}
	m.ruleForm = ruleFormState{}
	m.profileForm = profileFormState{}
	m.providerForm = providerFormState{}
	m.jsonImportForm = jsonImportFormState{}
}

func (m model) breadcrumbs() []breadcrumb {
	switch m.tab {
	case tabServers:
		items := []breadcrumb{
			{
				label: "Сервера",
				action: func(m *model) {
					m.closeAllForms()
					m.filter = ""
					m.clampTreeScroll()
				},
			},
		}
		if m.subForm.active {
			items = append(items, breadcrumb{label: "Новая подписка"})
		} else if m.searching || m.filter != "" {
			lbl := "Поиск"
			if m.filter != "" {
				lbl = "Поиск: " + m.filter
			}
			items = append(items, breadcrumb{label: lbl})
		}
		return items

	case tabSubs:
		items := []breadcrumb{
			{
				label: "Подписки",
				action: func(m *model) {
					m.closeAllForms()
				},
			},
		}
		if m.subForm.active {
			items = append(items, breadcrumb{label: "Новая подписка"})
		}
		return items

	case tabSettings:
		items := []breadcrumb{
			{
				label: "Настройки",
				action: func(m *model) {
					m.closeAllForms()
					m.routingView = routingNone
				},
			},
		}
		if m.inSettingsForm() {
			items = append(items, breadcrumb{label: settingsFormTitle(m.settingsForm.kind)})
			return items
		}

		switch m.routingView {
		case routingProfiles:
			items = append(items, breadcrumb{
				label: "Правила роутинга",
				action: func(m *model) {
					m.closeAllForms()
					m.routingView = routingProfiles
				},
			})
			if m.profileForm.active {
				items = append(items, breadcrumb{label: "Новый профиль"})
			} else if m.jsonImportForm.active {
				items = append(items, breadcrumb{label: "Импорт JSON"})
			}

		case routingProfileEdit:
			items = append(items, breadcrumb{
				label: "Правила роутинга",
				action: func(m *model) {
					m.closeAllForms()
					m.routingView = routingProfiles
				},
			})
			profName := "Профиль"
			if m.routingEditIdx >= 0 && m.routingEditIdx < len(m.store.Routing.Profiles) {
				if n := strings.TrimSpace(m.store.Routing.Profiles[m.routingEditIdx].Name); n != "" {
					profName = n
				}
			}
			items = append(items, breadcrumb{
				label: profName,
				action: func(m *model) {
					m.closeAllForms()
					m.routingView = routingProfileEdit
				},
			})
			if m.ruleForm.active {
				lbl := "Новое правило"
				if m.ruleForm.editIdx >= 0 {
					lbl = "Правка правила"
					if n := strings.TrimSpace(m.ruleForm.name); n != "" {
						lbl = n
					}
				}
				items = append(items, breadcrumb{label: lbl})
			} else if m.jsonImportForm.active {
				items = append(items, breadcrumb{label: "Импорт JSON"})
			}

		case routingProviders:
			items = append(items,
				breadcrumb{
					label: "Правила роутинга",
					action: func(m *model) {
						m.closeAllForms()
						m.routingView = routingProfiles
					},
				},
				breadcrumb{
					label: "Провайдеры геобаз",
					action: func(m *model) {
						m.closeAllForms()
						m.routingView = routingProviders
					},
				},
			)
			if m.providerForm.active {
				items = append(items, breadcrumb{label: "Новый провайдер"})
			}
		}
		return items

	case tabLogs:
		return []breadcrumb{
			{
				label: "Логи",
				action: func(m *model) {
					m.closeAllForms()
				},
			},
		}
	}
	return nil
}

func settingsFormTitle(kind settingsFormKind) string {
	switch kind {
	case sFormSocksPort:
		return "Порт SOCKS5"
	case sFormHTTPPort:
		return "Порт HTTP"
	case sFormAuth:
		return "Авторизация прокси"
	case sFormPingURL:
		return "URL пинга (HEAD)"
	case sFormHWID:
		return "HWID"
	case sFormUA:
		return "User-Agent"
	default:
		return "Ввод"
	}
}

func (m model) layoutBreadcrumbs(w int) ([]string, []crumbBox) {
	crumbs := m.breadcrumbs()
	if len(crumbs) == 0 {
		return []string{""}, nil
	}

	limitX := maxInt(12, w-1)
	maxSegW := maxInt(6, limitX-3)
	sepFull := styleMuted.Render(" / ")
	sepEnd := styleMuted.Render(" /")
	sepStart := styleMuted.Render("/ ")
	activeStyle := styleAccent.Bold(true)

	var lines []string
	var boxes []crumbBox
	var cur strings.Builder
	cur.WriteByte(' ')
	curX := 1
	lineIdx := 0

	// Стрелка ← показывается, когда мы глубже корня вкладки, и кликабельна как «назад»
	if len(crumbs) > 1 {
		cur.WriteString(styleMuted.Render("← "))
		boxes = append(boxes, crumbBox{
			y:   0,
			x1:  1,
			x2:  3,
			idx: len(crumbs) - 2,
		})
		curX = 3
	}

	flushLine := func() {
		lines = append(lines, cur.String())
		cur.Reset()
		cur.WriteByte(' ')
		curX = 1
		lineIdx++
	}

	for i, c := range crumbs {
		seg := clipLine(c.label, maxSegW)
		segW := lipgloss.Width(seg)

		if i > 0 {
			if curX+3+segW <= limitX {
				cur.WriteString(sepFull)
				curX += 3
			} else if curX+2 <= limitX {
				cur.WriteString(sepEnd)
				flushLine()
			} else {
				flushLine()
				cur.WriteString(sepStart)
				curX += 2
				if curX+segW > limitX {
					seg = clipLine(c.label, maxInt(4, limitX-curX))
					segW = lipgloss.Width(seg)
				}
			}
		}

		startX := curX
		if i == len(crumbs)-1 {
			cur.WriteString(activeStyle.Render(seg))
		} else {
			cur.WriteString(seg)
		}
		curX += segW

		boxes = append(boxes, crumbBox{
			y:   lineIdx,
			x1:  startX,
			x2:  curX,
			idx: i,
		})
	}

	lines = append(lines, cur.String())
	return lines, boxes
}

func (m model) renderBreadcrumbs(w int) string {
	lines, _ := m.layoutBreadcrumbs(w)
	return strings.Join(lines, "\n")
}

func (m model) breadcrumbHeight(w int) int {
	lines, _ := m.layoutBreadcrumbs(w)
	return len(lines)
}