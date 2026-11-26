package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"xray-tui/internal/config"
	"xray-tui/internal/geo"
	"xray-tui/internal/parser"
	"xray-tui/internal/uiutil"
)

type routingScreen int

const (
	routingNone routingScreen = iota
	routingProfiles
	routingProfileEdit
	routingProviders
)

type geoDownloadDoneMsg struct {
	providerName string
	status       geo.Status
	err          error
}

type ruleFormState struct {
	active    bool
	editIdx   int // -1 = новое правило, >= 0 = редактирование существующего
	step      int // 0=название, 1=тип поля, 2=значения, 3=действие
	name      string
	matchType int
	value     string
	outbound  int
}

type profileFormState struct {
	active bool
	name   string
}

type providerFormState struct {
	active     bool
	step       int // 0=название, 1=geosite URL, 2=geoip URL
	name       string
	geositeURL string
	geoipURL   string
}

type jsonImportFormState struct {
	active      bool
	asNew       bool // true = создать новый профиль, false = импорт в текущий
	step        int  // 0 = имя (если asNew) или режим (если !asNew), 1 = JSON или путь к файлу
	name        string
	replace     bool
	input       string
	inputPasted bool
}

var ruleFieldTypes = []struct {
	key   string
	label string
	hint  string
}{
	{"domain", "domain", "geosite:private, geosite:porn"},
	{"ip", "ip", "geoip:private, 0.0.0.0/0"},
	{"protocol", "protocol", "bittorrent, quic, http, tls"},
	{"port", "port", "53, 80, 443, 1000-2000"},
	{"network", "network", "tcp, udp"},
	{"source", "source IP", "192.168.1.0/24, 10.0.0.2"},
	{"sourcePort", "sourcePort", "1000-2000, 443"},
	{"localIP", "localIP", "127.0.0.1, 172.19.0.1"},
	{"localPort", "localPort", "10808, 53"},
	{"inboundTag", "inboundTag", "tun-in, socks-in"},
	{"attrs", "attrs", "user-agent=curl, :method=GET"},
}

var outboundNames = [3]string{"direct", "proxy", "block"}

func (m model) inRoutingForm() bool {
	return m.ruleForm.active || m.profileForm.active || m.providerForm.active || m.jsonImportForm.active
}

func (m model) handleRoutingKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.ruleForm.active {
		return m.handleRuleFormKey(msg)
	}
	if m.jsonImportForm.active {
		return m.handleJSONImportFormKey(msg)
	}
	if m.profileForm.active {
		return m.handleProfileFormKey(msg)
	}
	if m.providerForm.active {
		return m.handleProviderFormKey(msg)
	}

	switch m.routingView {
	case routingProfiles:
		return m.handleProfilesListKey(msg.String())
	case routingProfileEdit:
		return m.handleProfileEditKey(msg.String())
	case routingProviders:
		return m.handleProvidersKey(msg.String())
	}
	return m, nil
}

// --- Экран 1: Список профилей роутинга ---

func (m model) handleProfilesListKey(key string) (tea.Model, tea.Cmd) {
	total := 2 + len(m.store.Routing.Profiles)

	switch key {
	case "esc", "backspace":
		m.routingView = routingNone
		return m, nil
	case "up", "k":
		if m.routingCursor > 0 {
			m.routingCursor--
		}
	case "i":
		m.jsonImportForm = jsonImportFormState{active: true, asNew: true}
	case "e":
		idx := m.routingCursor - 2
		if idx >= 0 && idx < len(m.store.Routing.Profiles) {
			path, err := exportProfileRulesJSON(m.store.Routing.Profiles[idx])
			if err != nil {
				m.notice = "Ошибка экспорта: " + err.Error()
				m.noticeErr = true
			} else {
				m.notice = "Экспорт: " + shortHomePath(path)
				m.noticeErr = false
			}
		}
	case "down", "j":
		if m.routingCursor < total-1 {
			m.routingCursor++
		}
	case "enter", "right", "l":
		switch m.routingCursor {
		case 0:
			m.store.Routing.Enabled = !m.store.Routing.Enabled
			_ = m.store.Save()
			m.setRoutingNotice("Роутинг переключён")
		case 1:
			m.routingView = routingProviders
			m.routingProvCursor = 0
		default:
			idx := m.routingCursor - 2
			if idx >= 0 && idx < len(m.store.Routing.Profiles) {
				m.routingEditIdx = idx
				m.routingRuleCursor = 0
				m.routingView = routingProfileEdit
			}
		}
	case " ":
		if m.routingCursor == 0 {
			m.store.Routing.Enabled = !m.store.Routing.Enabled
			_ = m.store.Save()
			m.setRoutingNotice("Роутинг переключён")
		} else if idx := m.routingCursor - 2; idx >= 0 && idx < len(m.store.Routing.Profiles) {
			m.store.Routing.ActiveProfileID = m.store.Routing.Profiles[idx].ID
			m.store.Routing.Enabled = true
			_ = m.store.Save()
			m.setRoutingNotice("Выбран профиль: " + m.store.Routing.Profiles[idx].Name)
		}
	case "a":
		m.profileForm = profileFormState{active: true}
	case "d", "x":
		idx := m.routingCursor - 2
		if idx >= 0 && idx < len(m.store.Routing.Profiles) {
			if len(m.store.Routing.Profiles) <= 1 {
				m.notice = "Нельзя удалить последний профиль"
				m.noticeErr = true
				return m, nil
			}
			removed := m.store.Routing.Profiles[idx]
			m.store.Routing.Profiles = append(m.store.Routing.Profiles[:idx], m.store.Routing.Profiles[idx+1:]...)
			if m.store.Routing.ActiveProfileID == removed.ID {
				m.store.Routing.ActiveProfileID = m.store.Routing.Profiles[0].ID
			}
			if m.routingCursor >= 2+len(m.store.Routing.Profiles) {
				m.routingCursor--
			}
			_ = m.store.Save()
			m.setRoutingNotice("Удалён профиль: " + removed.Name)
		}
	case "r":
		return m.applyRoutingNow()
	}
	return m, nil
}

// --- Экран 2: Редактор выбранного профиля ---

func (m model) handleProfileEditKey(key string) (tea.Model, tea.Cmd) {
	if m.routingEditIdx < 0 || m.routingEditIdx >= len(m.store.Routing.Profiles) {
		m.routingView = routingProfiles
		return m, nil
	}
	rp := &m.store.Routing.Profiles[m.routingEditIdx]
	const headerItems = 5
	total := headerItems + len(rp.Rules)

	switch key {
	case "esc", "backspace":
		m.routingView = routingProfiles
		return m, nil
	case "up", "k":
		if m.routingRuleCursor > 0 {
			m.routingRuleCursor--
		}
	case "down", "j":
		if m.routingRuleCursor < total-1 {
			m.routingRuleCursor++
		}
	case "i":
		m.jsonImportForm = jsonImportFormState{active: true, asNew: false, replace: true}
	case "E":
		path, err := exportProfileRulesJSON(*rp)
		if err != nil {
			m.notice = "Ошибка экспорта: " + err.Error()
			m.noticeErr = true
		} else {
			m.notice = "Экспорт: " + shortHomePath(path)
			m.noticeErr = false
		}
	case "g":
		m.routingView = routingProviders
		m.routingProvCursor = 0
		return m, nil

	case "e":
		idx := m.routingRuleCursor - headerItems
		if idx >= 0 && idx < len(rp.Rules) {
			m.ruleForm = ruleFormFromExisting(idx, rp.Rules[idx])
		}

	case " ":
		if m.routingRuleCursor >= headerItems {
			idx := m.routingRuleCursor - headerItems
			if idx >= 0 && idx < len(rp.Rules) {
				rp.Rules[idx].Enabled = !rp.Rules[idx].Enabled
				_ = m.store.Save()
				m.setRoutingNotice("Правило: " + onOffLabel(rp.Rules[idx].Enabled))
			}
			return m, nil
		}
		return m.handleProfileHeaderAction(rp, key)

	case "enter", "left", "right", "h", "l":
		if m.routingRuleCursor < headerItems {
			return m.handleProfileHeaderAction(rp, key)
		}
		idx := m.routingRuleCursor - headerItems
		if idx >= 0 && idx < len(rp.Rules) {
			if key == "enter" {
				m.ruleForm = ruleFormFromExisting(idx, rp.Rules[idx])
			} else {
				rp.Rules[idx].Outbound = cycleString(outboundNames[:], rp.Rules[idx].Outbound, key == "left" || key == "h")
				_ = m.store.Save()
				m.setRoutingNotice("Действие: " + strings.ToUpper(rp.Rules[idx].Outbound))
			}
		}

	case "o":
		idx := m.routingRuleCursor - headerItems
		if idx >= 0 && idx < len(rp.Rules) {
			rp.Rules[idx].Outbound = cycleString(outboundNames[:], rp.Rules[idx].Outbound, false)
			_ = m.store.Save()
			m.setRoutingNotice("Действие: " + strings.ToUpper(rp.Rules[idx].Outbound))
		}

	case "shift+up", "ctrl+up", "K":
		idx := m.routingRuleCursor - headerItems
		if idx > 0 && idx < len(rp.Rules) {
			rp.Rules[idx-1], rp.Rules[idx] = rp.Rules[idx], rp.Rules[idx-1]
			m.routingRuleCursor--
			_ = m.store.Save()
			m.setRoutingNotice("Поднято: " + rp.Rules[idx-1].Name)
		}

	case "shift+down", "ctrl+down", "J":
		idx := m.routingRuleCursor - headerItems
		if idx >= 0 && idx < len(rp.Rules)-1 {
			rp.Rules[idx+1], rp.Rules[idx] = rp.Rules[idx], rp.Rules[idx+1]
			m.routingRuleCursor++
			_ = m.store.Save()
			m.setRoutingNotice("Опущено: " + rp.Rules[idx+1].Name)
		}

	case "d", "x":
		idx := m.routingRuleCursor - headerItems
		if idx >= 0 && idx < len(rp.Rules) {
			name := rp.Rules[idx].Name
			rp.Rules = append(rp.Rules[:idx], rp.Rules[idx+1:]...)
			if m.routingRuleCursor >= headerItems+len(rp.Rules) && m.routingRuleCursor > 0 {
				m.routingRuleCursor--
			}
			_ = m.store.Save()
			m.setRoutingNotice("Удалено: " + name)
		}

	case "a":
		m.ruleForm = ruleFormState{active: true, editIdx: -1}

	case "R":
		rp.Rules = config.DefaultRoutingRules()
		_ = m.store.Save()
		m.routingRuleCursor = headerItems
		m.setRoutingNotice("Сброшено к стандартным правилам")

	case "r":
		return m.applyRoutingNow()
	}

	return m, nil
}

func (m model) handleProfileHeaderAction(rp *config.RoutingProfile, key string) (tea.Model, tea.Cmd) {
	rev := key == "left" || key == "h"
	switch m.routingRuleCursor {
	case 0:
		m.store.Routing.ActiveProfileID = rp.ID
		m.store.Routing.Enabled = true
		_ = m.store.Save()
		m.setRoutingNotice("Активный профиль: " + rp.Name)
		// Смена активного профиля — сразу применяем к работающему прокси.
		return m.applyRoutingNowKeep()
	case 1:
		strategies := []string{"AsIs", "IPIfNonMatch", "IPOnDemand"}
		rp.DomainStrategy = cycleString(strategies, rp.DomainStrategy, rev)
		_ = m.store.Save()
		m.setRoutingNotice("Strategy: " + rp.DomainStrategy)
	case 2:
		modes := []string{"prepend", "append", "replace", "off"}
		rp.RawJSONMode = cycleString(modes, rp.RawJSONMode, rev)
		_ = m.store.Save()
		m.setRoutingNotice("Raw JSON: " + rawModeLabel(rp.RawJSONMode))
	case 3:
		if len(m.store.Routing.GeoProviders) > 0 {
			ids := make([]string, len(m.store.Routing.GeoProviders))
			for i, p := range m.store.Routing.GeoProviders {
				ids[i] = p.ID
			}
			rp.GeoProviderID = cycleString(ids, rp.GeoProviderID, rev)
			_ = m.store.Save()
			if prov := m.store.FindGeoProvider(rp.GeoProviderID); prov != nil {
				m.setRoutingNotice("Геобазы: " + prov.Name)
			}
		}
	case 4:
		if key == "enter" || key == " " {
			if prov := m.store.FindGeoProvider(rp.GeoProviderID); prov != nil && !m.busy {
				m.busy = true
				m.notice = "Скачиваю геобазы (" + prov.Name + ")..."
				m.noticeErr = false
				return m, downloadGeoCmd(*prov)
			}
		}
	}
	return m, nil
}

func ruleFormFromExisting(idx int, r config.RoutingRule) ruleFormState {
	f := ruleFormState{
		active:  true,
		editIdx: idx,
		name:    r.Name,
	}

	for i, o := range outboundNames {
		if o == r.Outbound {
			f.outbound = i
			break
		}
	}

	switch {
	case len(r.Domains) > 0:
		f.matchType = 0
		f.value = strings.Join(r.Domains, ", ")
	case len(r.IPs) > 0:
		f.matchType = 1
		f.value = strings.Join(r.IPs, ", ")
	case len(r.Protocol) > 0:
		f.matchType = 2
		f.value = strings.Join(r.Protocol, ", ")
	case r.Port != "":
		f.matchType = 3
		f.value = r.Port
	case r.Network != "":
		f.matchType = 4
		f.value = r.Network
	case len(r.Source) > 0:
		f.matchType = 5
		f.value = strings.Join(r.Source, ", ")
	case r.SourcePort != "":
		f.matchType = 6
		f.value = r.SourcePort
	case len(r.LocalIP) > 0:
		f.matchType = 7
		f.value = strings.Join(r.LocalIP, ", ")
	case r.LocalPort != "":
		f.matchType = 8
		f.value = r.LocalPort
	case len(r.InboundTag) > 0:
		f.matchType = 9
		f.value = strings.Join(r.InboundTag, ", ")
	case len(r.Attrs) > 0:
		f.matchType = 10
		keys := make([]string, 0, len(r.Attrs))
		for k := range r.Attrs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, k+"="+r.Attrs[k])
		}
		f.value = strings.Join(pairs, ", ")
	}
	return f
}

// --- Экран 3: Провайдеры геобаз ---

func (m model) handleProvidersKey(key string) (tea.Model, tea.Cmd) {
	n := len(m.store.Routing.GeoProviders)
	switch key {
	case "esc", "backspace":
		m.routingView = routingProfiles
		return m, nil
	case "up", "k":
		if m.routingProvCursor > 0 {
			m.routingProvCursor--
		}
	case "down", "j":
		if m.routingProvCursor < n-1 {
			m.routingProvCursor++
		}
	case "enter", "u":
		if !m.busy && n > 0 && m.routingProvCursor < n {
			prov := m.store.Routing.GeoProviders[m.routingProvCursor]
			m.busy = true
			m.notice = "Скачиваю геобазы (" + prov.Name + ")..."
			m.noticeErr = false
			return m, downloadGeoCmd(prov)
		}
	case "a":
		m.providerForm = providerFormState{active: true}
	case "d", "x":
		if n <= 1 {
			m.notice = "Нельзя удалить последнего провайдера"
			m.noticeErr = true
			return m, nil
		}
		if m.routingProvCursor >= 0 && m.routingProvCursor < n {
			removed := m.store.Routing.GeoProviders[m.routingProvCursor]
			m.store.Routing.GeoProviders = append(m.store.Routing.GeoProviders[:m.routingProvCursor], m.store.Routing.GeoProviders[m.routingProvCursor+1:]...)
			fallbackID := m.store.Routing.GeoProviders[0].ID
			for i := range m.store.Routing.Profiles {
				if m.store.Routing.Profiles[i].GeoProviderID == removed.ID {
					m.store.Routing.Profiles[i].GeoProviderID = fallbackID
				}
			}
			if m.routingProvCursor >= len(m.store.Routing.GeoProviders) {
				m.routingProvCursor--
			}
			_ = m.store.Save()
			m.notice = "Удалён провайдер: " + removed.Name
			m.noticeErr = false
		}
	}
	return m, nil
}

// --- Формы ввода ---

func (m model) handleProfileFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.profileForm = profileFormState{}
	case "enter":
		name := strings.TrimSpace(m.profileForm.name)
		if name == "" {
			m.notice = "Введи название профиля"
			m.noticeErr = true
			return m, nil
		}
		defGeo := config.DefaultGeoProviderID
		if len(m.store.Routing.GeoProviders) > 0 {
			defGeo = m.store.Routing.GeoProviders[0].ID
		}
		prof := config.RoutingProfile{
			ID:             config.NewID(),
			Name:           name,
			DomainStrategy: "IPIfNonMatch",
			RawJSONMode:    "prepend",
			GeoProviderID:  defGeo,
			Rules:          config.DefaultRoutingRules(),
		}
		m.store.Routing.Profiles = append(m.store.Routing.Profiles, prof)
		_ = m.store.Save()
		m.profileForm = profileFormState{}
		m.routingEditIdx = len(m.store.Routing.Profiles) - 1
		m.routingRuleCursor = 0
		m.routingView = routingProfileEdit
		m.notice = "Создан профиль: " + name
		m.noticeErr = false
	case "backspace":
		m.profileForm.name = trimLastRune(m.profileForm.name)
	default:
		if add := extractInputText(msg); add != "" {
			m.profileForm.name += add
		}
	}
	return m, nil
}

func (m model) handleProviderFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.providerForm = providerFormState{}
	case "up":
		if m.providerForm.step > 0 {
			m.providerForm.step--
		}
	case "down":
		if m.providerForm.step < 2 {
			m.providerForm.step++
		}
	case "enter":
		if m.providerForm.step < 2 {
			m.providerForm.step++
			return m, nil
		}
		name := strings.TrimSpace(m.providerForm.name)
		siteURL := strings.TrimSpace(m.providerForm.geositeURL)
		ipURL := strings.TrimSpace(m.providerForm.geoipURL)
		if name == "" || siteURL == "" || ipURL == "" {
			m.notice = "Заполни имя и обе ссылки (.dat)"
			m.noticeErr = true
			return m, nil
		}
		prov := config.GeoProvider{
			ID:         config.NewID(),
			Name:       name,
			GeositeURL: siteURL,
			GeoipURL:   ipURL,
		}
		m.store.Routing.GeoProviders = append(m.store.Routing.GeoProviders, prov)
		_ = m.store.Save()
		m.providerForm = providerFormState{}
		m.routingProvCursor = len(m.store.Routing.GeoProviders) - 1
		m.notice = "Добавлен провайдер: " + name + " (Enter — скачать)"
		m.noticeErr = false
	case "backspace":
		switch m.providerForm.step {
		case 0:
			m.providerForm.name = trimLastRune(m.providerForm.name)
		case 1:
			m.providerForm.geositeURL = trimLastRune(m.providerForm.geositeURL)
		case 2:
			m.providerForm.geoipURL = trimLastRune(m.providerForm.geoipURL)
		}
	default:
		if add := extractInputText(msg); add != "" {
			switch m.providerForm.step {
			case 0:
				m.providerForm.name += add
			case 1:
				m.providerForm.geositeURL += add
			case 2:
				m.providerForm.geoipURL += add
			}
		}
	}
	return m, nil
}

func (m model) handleRuleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.ruleForm = ruleFormState{}
	case "up":
		if m.ruleForm.step > 0 {
			m.ruleForm.step--
		}
	case "down":
		if m.ruleForm.step < 3 {
			m.ruleForm.step++
		}
	case "ctrl+s":
		return m.commitRuleFormApplied()
	case "enter":
		if m.ruleForm.step < 3 {
			if m.ruleForm.step == 2 && strings.TrimSpace(m.ruleForm.value) == "" {
				m.notice = "Введи хотя бы одно значение"
				m.noticeErr = true
				return m, nil
			}
			m.ruleForm.step++
			return m, nil
		}
		return m.commitRuleFormApplied()
	case "tab", "right":
		if m.ruleForm.step == 1 {
			m.ruleForm.matchType = (m.ruleForm.matchType + 1) % len(ruleFieldTypes)
		} else if m.ruleForm.step == 3 {
			m.ruleForm.outbound = (m.ruleForm.outbound + 1) % 3
		}
	case "shift+tab", "left":
		if m.ruleForm.step == 1 {
			m.ruleForm.matchType = (m.ruleForm.matchType + len(ruleFieldTypes) - 1) % len(ruleFieldTypes)
		} else if m.ruleForm.step == 3 {
			m.ruleForm.outbound = (m.ruleForm.outbound + 2) % 3
		}
	case "backspace":
		if m.ruleForm.step == 0 {
			m.ruleForm.name = trimLastRune(m.ruleForm.name)
		} else if m.ruleForm.step == 2 {
			m.ruleForm.value = trimLastRune(m.ruleForm.value)
		}
	default:
		if m.ruleForm.step == 0 || m.ruleForm.step == 2 {
			if add := extractInputText(msg); add != "" {
				if m.ruleForm.step == 0 {
					m.ruleForm.name += add
				} else {
					m.ruleForm.value += add
				}
			}
		}
	}
	return m, nil
}

func (m model) commitRuleForm() model {
	if m.routingEditIdx < 0 || m.routingEditIdx >= len(m.store.Routing.Profiles) {
		m.ruleForm = ruleFormState{}
		return m
	}

	rawVals := strings.Split(m.ruleForm.value, ",")
	vals := make([]string, 0, len(rawVals))
	for _, v := range rawVals {
		if t := strings.TrimSpace(v); t != "" {
			vals = append(vals, t)
		}
	}
	if len(vals) == 0 {
		m.notice = "Значение не может быть пустым"
		m.noticeErr = true
		return m
	}

	name := strings.TrimSpace(m.ruleForm.name)
	if name == "" {
		name = vals[0]
	}

	rp := &m.store.Routing.Profiles[m.routingEditIdx]
	isEdit := m.ruleForm.editIdx >= 0 && m.ruleForm.editIdx < len(rp.Rules)

	rule := config.RoutingRule{
		ID:       config.NewID(),
		Name:     name,
		Enabled:  true,
		Outbound: outboundNames[m.ruleForm.outbound],
	}
	if isEdit {
		rule.ID = rp.Rules[m.ruleForm.editIdx].ID
		rule.Enabled = rp.Rules[m.ruleForm.editIdx].Enabled
	}

	switch ruleFieldTypes[m.ruleForm.matchType].key {
	case "domain":
		rule.Domains = vals
	case "ip":
		rule.IPs = vals
	case "protocol":
		rule.Protocol = vals
	case "port":
		rule.Port = strings.Join(vals, ",")
	case "network":
		rule.Network = strings.Join(vals, ",")
	case "source":
		rule.Source = vals
	case "sourcePort":
		rule.SourcePort = strings.Join(vals, ",")
	case "localIP":
		rule.LocalIP = vals
	case "localPort":
		rule.LocalPort = strings.Join(vals, ",")
	case "inboundTag":
		rule.InboundTag = vals
	case "attrs":
		attrs := make(map[string]string, len(vals))
		for _, pair := range vals {
			k, v, ok := strings.Cut(pair, "=")
			if ok && strings.TrimSpace(k) != "" {
				attrs[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
		}
		if len(attrs) == 0 {
			m.notice = "Для attrs используй формат ключ=значение"
			m.noticeErr = true
			return m
		}
		rule.Attrs = attrs
	}

	if isEdit {
		rp.Rules[m.ruleForm.editIdx] = rule
		m.routingRuleCursor = 5 + m.ruleForm.editIdx
		m.setRoutingNotice("Сохранено: " + rule.Name)
	} else {
		rp.Rules = append(rp.Rules, rule)
		m.routingRuleCursor = 5 + len(rp.Rules) - 1
		m.setRoutingNotice("Добавлено: " + rule.Name)
	}

	_ = m.store.Save()
	m.ruleForm = ruleFormState{}
	return m
}

func (m model) applyRoutingNow() (tea.Model, tea.Cmd) {
	if m.status != nil && m.status.Running {
		if active := m.store.ActiveProfile(); active != nil {
			m.busy = true
			m.notice = "Применяю роутинг..."
			m.noticeErr = false
			return m, m.connectProfileCmd(*active)
		}
	}
	m.notice = "Сохранено (применится при подключении)"
	m.noticeErr = false
	return m, nil
}

// applyRoutingNowKeep делает то же, что applyRoutingNow, но не затирает уже
// выставленное уведомление (например, «Добавлено: правило»).
func (m model) applyRoutingNowKeep() (tea.Model, tea.Cmd) {
	if m.status != nil && m.status.Running {
		if active := m.store.ActiveProfile(); active != nil {
			m.busy = true
			m.notice += " · применяю..."
			m.noticeErr = false
			return m, m.connectProfileCmd(*active)
		}
	}
	return m, nil
}

// commitRuleFormApplied сохраняет правило и сразу применяет конфиг к работающему
// прокси — ручное «r» для этого больше не нужно.
func (m model) commitRuleFormApplied() (tea.Model, tea.Cmd) {
	m = m.commitRuleForm()
	if m.noticeErr {
		return m, nil
	}
	return m.applyRoutingNowKeep()
}

func (m *model) setRoutingNotice(base string) {
	if m.status != nil && m.status.Running {
		m.notice = base + " (r — применить)"
	} else {
		m.notice = base
	}
	m.noticeErr = false
}

func downloadGeoCmd(prov config.GeoProvider) tea.Cmd {
	return func() tea.Msg {
		dir := config.GeoProviderDir(prov.ID)
		st, err := geo.Download(context.Background(), prov, dir)
		return geoDownloadDoneMsg{
			providerName: prov.Name,
			status:       st,
			err:          err,
		}
	}
}

// --- Отрисовка экранов роутинга ---

func (m model) renderRoutingScreen(w, h int) string {
	if m.jsonImportForm.active {
		return m.renderJSONImportForm(w, h)
	}
	if m.ruleForm.active {
		return m.renderRuleForm(w, h)
	}
	if m.profileForm.active {
		return m.renderProfileForm(w, h)
	}
	if m.providerForm.active {
		return m.renderProviderForm(w, h)
	}

	switch m.routingView {
	case routingProfiles:
		return m.renderProfilesList(w, h)
	case routingProfileEdit:
		return m.renderProfileEdit(w, h)
	case routingProviders:
		return m.renderProvidersList(w, h)
	default:
		return ""
	}
}

func (m model) renderProfilesList(w, h int) string {
	innerW := maxInt(20, w-4)

	var lines []string

	provVal := fmt.Sprintf("%d шт. →", len(m.store.Routing.GeoProviders))
	topBox := renderBox("Управление", []string{
		renderSettingRow("Роутинг", onOffLabel(m.store.Routing.Enabled), m.routingCursor == 0, innerW),
		renderSettingRow("Провайдеры геобаз", provVal, m.routingCursor == 1, innerW),
	}, w)
	lines = append(lines, strings.Split(topBox, "\n")...)
	lines = append(lines, clipLine(" "+styleMuted.Render("Профили (Space — выбрать, Enter — открыть):"), w))

	for i, rp := range m.store.Routing.Profiles {
		selected := m.routingCursor == 2+i
		prefix := "  "
		if selected {
			prefix = styleAccent.Render("▸ ")
		}
		dot := styleMuted.Render("○ ")
		if rp.ID == m.store.Routing.ActiveProfileID {
			dot = styleGreen.Render("● ")
		}

		geoName := rp.GeoProviderID
		if p := m.store.FindGeoProvider(rp.GeoProviderID); p != nil {
			geoName = uiutil.Display(p.Name)
		}
		right := styleMuted.Render(fmt.Sprintf(" %d прав. · %s", len(rp.Rules), geoName))
		leftW := termWidth(prefix) + termWidth(dot)
		maxName := maxInt(6, w-leftW-termWidth(right)-1)

		name := clipLine(uiutil.Display(rp.Name), maxName)
		if selected {
			name = styleCursorRow.Render(name)
		}
		gap := maxInt(0, w-leftW-termWidth(name)-termWidth(right))
		row := clipLine(prefix+dot+name+strings.Repeat(" ", gap)+right, w)
		if selected {
			row = highlightRow(row, w)
		}
		lines = append(lines, row)
	}

	return padLines(lines, h)
}

func (m model) renderProfileEdit(w, h int) string {
	if m.routingEditIdx < 0 || m.routingEditIdx >= len(m.store.Routing.Profiles) {
		return padLines(nil, h)
	}
	rp := m.store.Routing.Profiles[m.routingEditIdx]
	innerW := maxInt(20, w-4)

	var lines []string

	activeVal := "Выбрать"
	if rp.ID == m.store.Routing.ActiveProfileID {
		activeVal = "Да ●"
	}

	provName := "—"
	if p := m.store.FindGeoProvider(rp.GeoProviderID); p != nil {
		provName = uiutil.Display(p.Name)
	}

	geoSt := geo.Check(config.GeoProviderDir(rp.GeoProviderID))
	geoVal := "Скачать ⇣"
	if geoSt.Ready() {
		geoVal = "Обн. (" + formatBytesShort(geoSt.TotalBytes) + ")"
	}

	topBox := renderBox("Параметры профиля", []string{
		renderSettingRow("Активный профиль", activeVal, m.routingRuleCursor == 0, innerW),
		renderSettingRow("Domain Strategy", rp.DomainStrategy, m.routingRuleCursor == 1, innerW),
		renderSettingRow("Для Raw JSON", rawModeLabel(rp.RawJSONMode), m.routingRuleCursor == 2, innerW),
		renderSettingRow("Провайдер геобаз", provName, m.routingRuleCursor == 3, innerW),
		renderSettingRow("Файлы геобаз", geoVal, m.routingRuleCursor == 4, innerW),
	}, w)
	lines = append(lines, strings.Split(topBox, "\n")...)
	lines = append(lines, clipLine(" "+styleMuted.Render(fmt.Sprintf("Правила (%d) · Shift+↑↓ перетащить:", len(rp.Rules))), w))

	avail := maxInt(2, h-len(lines))
	if len(rp.Rules) == 0 {
		lines = append(lines, styleMuted.Render("  Правил нет. Нажми 'a' чтобы добавить."))
		return padLines(lines, h)
	}

	const headerItems = 5
	start := 0
	selRule := m.routingRuleCursor - headerItems
	if selRule >= avail {
		start = selRule - avail + 1
	}
	end := start + avail
	if end > len(rp.Rules) {
		end = len(rp.Rules)
	}

	for i := start; i < end; i++ {
		r := rp.Rules[i]
		selected := m.routingRuleCursor == headerItems+i

		prefix := "  "
		if selected {
			prefix = styleAccent.Render("▸ ")
		}
		chk := styleMuted.Render("[ ] ")
		if r.Enabled {
			chk = styleGreen.Render("[x] ")
		}
		badge := formatOutboundBadge(r.Outbound)
		summary := summarizeRule(r)

		leftW := termWidth(prefix) + termWidth(chk) + termWidth(badge) + 1
		maxText := maxInt(8, w-leftW-1)

		ruleName := uiutil.Display(r.Name)
		label := ruleName
		if summary != "" {
			label = ruleName + " " + styleMuted.Render("("+summary+")")
		}
		if selected {
			label = styleCursorRow.Render(clipLine(ruleName+" ("+summary+")", maxText))
		} else {
			label = clipLine(label, maxText)
		}
		row := clipLine(prefix+chk+badge+" "+label, w)
		if selected {
			row = highlightRow(row, w)
		}
		lines = append(lines, row)
	}

	return padLines(lines, h)
}

func (m model) renderProvidersList(w, h int) string {
	var lines []string
	lines = append(lines, "")

	for i, p := range m.store.Routing.GeoProviders {
		selected := i == m.routingProvCursor
		prefix := "  "
		name := styleTitle.Render(p.Name)
		if selected {
			prefix = styleAccent.Render("▸ ")
			name = styleCursorRow.Render(p.Name)
		}

		st := geo.Check(config.GeoProviderDir(p.ID))
		badge := styleRed.Render("не скачаны")
		if st.Ready() {
			badge = styleGreen.Render(fmt.Sprintf("%s · %s", formatBytesShort(st.TotalBytes), st.UpdatedAt.Format("02.01 15:04")))
		}

		headerLine := clipLine(prefix+name+"  "+badge, w)
		if selected {
			headerLine = highlightRow(headerLine, w)
		}
		lines = append(lines,
			headerLine,
			clipLine(styleMuted.Render("   site: ")+p.GeositeURL, w),
			clipLine(styleMuted.Render("   ip:   ")+p.GeoipURL, w),
			"",
		)
	}

	return padLines(lines, h)
}

func (m model) renderProfileForm(w, h int) string {
	innerW := maxInt(20, w-4)
	rows := wrapField("Название", m.profileForm.name+styleAccent.Render("█"), true, innerW)
	lines := append([]string{""}, strings.Split(renderBox("Новый профиль роутинга", rows, w), "\n")...)
	lines = append(lines, "", clipLine(styleMuted.Render("  Enter — создать · Esc — отмена"), w))
	return padLines(lines, h)
}

func (m model) renderProviderForm(w, h int) string {
	innerW := maxInt(20, w-4)
	bar := styleAccent.Render("█")
	nVal, sVal, iVal := m.providerForm.name, m.providerForm.geositeURL, m.providerForm.geoipURL
	switch m.providerForm.step {
	case 0:
		nVal += bar
	case 1:
		sVal += bar
	case 2:
		iVal += bar
	}
	var rows []string
	rows = append(rows, wrapField("1. Имя", nVal, m.providerForm.step == 0, innerW)...)
	rows = append(rows, wrapField("2. geosite.dat", sVal, m.providerForm.step == 1, innerW)...)
	rows = append(rows, wrapField("3. geoip.dat", iVal, m.providerForm.step == 2, innerW)...)

	lines := append([]string{""}, strings.Split(renderBox("Новый провайдер геобаз", rows, w), "\n")...)
	lines = append(lines, "", clipLine(styleMuted.Render("  ↑↓ поля · Enter — далее/сохранить · Esc"), w))
	return padLines(lines, h)
}

func (m model) renderRuleForm(w, h int) string {
	innerW := maxInt(20, w-4)
	bar := styleAccent.Render("█")

	nameVal := m.ruleForm.name
	if m.ruleForm.step == 0 {
		nameVal += bar
	}
	valText := m.ruleForm.value
	if m.ruleForm.step == 2 {
		valText += bar
	}

	ft := ruleFieldTypes[m.ruleForm.matchType]
	var rows []string
	rows = append(rows, wrapField("1. Название", nameVal, m.ruleForm.step == 0, innerW)...)
	rows = append(rows, renderFormField("2. Тип (←/→)", "["+ft.label+"]", m.ruleForm.step == 1, innerW))
	rows = append(rows, wrapField("3. Значения", valText, m.ruleForm.step == 2, innerW)...)
	rows = append(rows, clipLine(styleMuted.Render("   пр.: "+ft.hint), innerW))
	rows = append(rows, renderFormField("4. Действие (←/→)", "["+strings.ToUpper(outboundNames[m.ruleForm.outbound])+"]", m.ruleForm.step == 3, innerW))

	title := "Новое правило роутинга"
	if m.ruleForm.editIdx >= 0 {
		title = "Редактирование правила"
	}

	boxLines := strings.Split(renderBox(title, rows, w), "\n")
	lines := append([]string{""}, boxLines...)
	lines = append(lines, "", clipLine(styleMuted.Render("  ↑↓ поля · Enter далее · Ctrl+S сохр · Esc"), w))

	if len(lines) > h {
		lines = lines[len(lines)-h:]
	}
	return padLines(lines, h)
}

// --- Вспомогательные ---

func renderFormField(label, value string, active bool, maxW int) string {
	prefix := "  "
	lbl := styleMuted.Render(label + ": ")
	if active {
		prefix = styleAccent.Render("▸ ")
		lbl = styleTitle.Render(label + ": ")
	}
	return clipLine(prefix+lbl+value, maxW)
}

func formatOutboundBadge(out string) string {
	switch out {
	case "direct":
		return styleGreen.Render("DIR")
	case "block":
		return styleRed.Render("BLK")
	default:
		return styleAccent.Render("PRX")
	}
}

func summarizeRule(r config.RoutingRule) string {
	var parts []string
	if len(r.Domains) > 0 {
		parts = append(parts, "dom:"+strings.Join(r.Domains, ","))
	}
	if len(r.IPs) > 0 {
		parts = append(parts, "ip:"+strings.Join(r.IPs, ","))
	}
	if len(r.Protocol) > 0 {
		parts = append(parts, "proto:"+strings.Join(r.Protocol, ","))
	}
	if r.Port != "" {
		parts = append(parts, "port:"+r.Port)
	}
	if r.Network != "" {
		parts = append(parts, "net:"+r.Network)
	}
	if len(r.Source) > 0 {
		parts = append(parts, "src:"+strings.Join(r.Source, ","))
	}
	if r.SourcePort != "" {
		parts = append(parts, "sport:"+r.SourcePort)
	}
	if len(r.LocalIP) > 0 {
		parts = append(parts, "lip:"+strings.Join(r.LocalIP, ","))
	}
	if r.LocalPort != "" {
		parts = append(parts, "lport:"+r.LocalPort)
	}
	if len(r.InboundTag) > 0 {
		parts = append(parts, "in:"+strings.Join(r.InboundTag, ","))
	}
	if len(r.Attrs) > 0 {
		parts = append(parts, fmt.Sprintf("attrs:%d", len(r.Attrs)))
	}
	return strings.Join(parts, " ")
}

func rawModeLabel(mode string) string {
	switch mode {
	case "append":
		return "В конец"
	case "replace":
		return "Заменить"
	case "off":
		return "Не трогать"
	default:
		return "В начало"
	}
}

func cycleString(list []string, cur string, reverse bool) string {
	idx := 0
	for i, v := range list {
		if v == cur {
			idx = i
			break
		}
	}
	if reverse {
		idx = (idx + len(list) - 1) % len(list)
	} else {
		idx = (idx + 1) % len(list)
	}
	return list[idx]
}

func extractInputText(msg tea.KeyMsg) string {
	if msg.Type == tea.KeyRunes {
		return sanitizeInput(string(msg.Runes))
	}
	if msg.String() == " " {
		return " "
	}
	return ""
}

func trimLastRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return ""
	}
	return string(r[:len(r)-1])
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// --- Импорт / экспорт правил в JSON ---

type externalRuleJSON struct {
	Enabled     *bool             `json:"enabled,omitempty"`
	Locked      bool              `json:"locked"`
	OutboundTag string            `json:"outboundTag"`
	Domain      []string          `json:"domain,omitempty"`
	IP          []string          `json:"ip,omitempty"`
	Port        string            `json:"port,omitempty"`
	Network     string            `json:"network,omitempty"`
	Protocol    []string          `json:"protocol,omitempty"`
	Source      []string          `json:"source,omitempty"`
	SourcePort  string            `json:"sourcePort,omitempty"`
	LocalIP     []string          `json:"localIP,omitempty"`
	LocalPort   string            `json:"localPort,omitempty"`
	InboundTag  []string          `json:"inboundTag,omitempty"`
	Attrs       map[string]string `json:"attrs,omitempty"`
	Remarks     string            `json:"remarks,omitempty"`
}

func (m model) handleJSONImportFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.jsonImportForm = jsonImportFormState{}
	case "up":
		if m.jsonImportForm.step > 0 {
			m.jsonImportForm.step--
		}
	case "down":
		if m.jsonImportForm.step < 1 {
			m.jsonImportForm.step++
		}
	case "tab", "left", "right":
		if !m.jsonImportForm.asNew && m.jsonImportForm.step == 0 {
			m.jsonImportForm.replace = !m.jsonImportForm.replace
		}
	case "ctrl+s":
		return m.commitJSONImport(), nil
	case "enter":
		if m.jsonImportForm.step == 0 {
			m.jsonImportForm.step = 1
			return m, nil
		}
		return m.commitJSONImport(), nil
	case "backspace":
		if m.jsonImportForm.step == 0 && m.jsonImportForm.asNew {
			m.jsonImportForm.name = trimLastRune(m.jsonImportForm.name)
		} else if m.jsonImportForm.step == 1 {
			m.jsonImportForm.input = trimLastRune(m.jsonImportForm.input)
		}
	default:
		if key == " " && !m.jsonImportForm.asNew && m.jsonImportForm.step == 0 {
			m.jsonImportForm.replace = !m.jsonImportForm.replace
			return m, nil
		}
		if add := extractInputText(msg); add != "" {
			if m.jsonImportForm.step == 0 && m.jsonImportForm.asNew {
				m.jsonImportForm.name += add
			} else if m.jsonImportForm.step == 1 {
				m.jsonImportForm.input += add
			}
		}
	}
	return m, nil
}

func (m model) commitJSONImport() model {
	rules, err := parseExternalRulesJSON(m.jsonImportForm.input)
	if err != nil {
		m.notice = err.Error()
		m.noticeErr = true
		return m
	}

	if m.jsonImportForm.asNew {
		name := strings.TrimSpace(m.jsonImportForm.name)
		if name == "" {
			name = fmt.Sprintf("Import (%d)", len(rules))
		}
		defGeo := config.DefaultGeoProviderID
		if len(m.store.Routing.GeoProviders) > 0 {
			defGeo = m.store.Routing.GeoProviders[0].ID
		}
		prof := config.RoutingProfile{
			ID:             config.NewID(),
			Name:           name,
			DomainStrategy: "IPIfNonMatch",
			RawJSONMode:    "prepend",
			GeoProviderID:  defGeo,
			Rules:          rules,
		}
		m.store.Routing.Profiles = append(m.store.Routing.Profiles, prof)
		_ = m.store.Save()
		m.jsonImportForm = jsonImportFormState{}
		m.routingEditIdx = len(m.store.Routing.Profiles) - 1
		m.routingRuleCursor = 0
		m.routingView = routingProfileEdit
		m.setRoutingNotice(fmt.Sprintf("Импортирован профиль %q (%d прав.)", name, len(rules)))
		return m
	}

	if m.routingEditIdx < 0 || m.routingEditIdx >= len(m.store.Routing.Profiles) {
		m.jsonImportForm = jsonImportFormState{}
		return m
	}
	rp := &m.store.Routing.Profiles[m.routingEditIdx]
	if m.jsonImportForm.replace {
		rp.Rules = rules
	} else {
		rp.Rules = append(rp.Rules, rules...)
	}
	_ = m.store.Save()
	m.jsonImportForm = jsonImportFormState{}
	m.routingRuleCursor = 5
	m.setRoutingNotice(fmt.Sprintf("Импортировано %d правил", len(rules)))
	return m
}

func (m model) renderJSONImportForm(w, h int) string {
	innerW := maxInt(20, w-4)
	bar := styleAccent.Render("█")

	var rows []string
	if m.jsonImportForm.asNew {
		nVal := m.jsonImportForm.name
		if m.jsonImportForm.step == 0 {
			nVal += bar
		}
		rows = append(rows, wrapField("1. Имя профиля", nVal, m.jsonImportForm.step == 0, innerW)...)
	} else {
		mode := "Добавить в конец"
		if m.jsonImportForm.replace {
			mode = "Заменить правила"
		}
		rows = append(rows, renderFormField("1. Режим (←/→)", "["+mode+"]", m.jsonImportForm.step == 0, innerW))
	}

	// Компактное отображение, если вставлен длинный JSON-массив
	inDisplay := fieldValue(m.jsonImportForm.input, m.jsonImportForm.inputPasted)
	if r := []rune(inDisplay); len(r) > 120 {
		inDisplay = fmt.Sprintf("[%d симв.] %s ... %s", len(r), string(r[:45]), string(r[len(r)-45:]))
	}
	if m.jsonImportForm.step == 1 {
		inDisplay += bar
	}
	rows = append(rows, wrapField("2. JSON / файл", inDisplay, m.jsonImportForm.step == 1, innerW)...)
	rows = append(rows, clipLine(styleMuted.Render("   Вставь [{...}] или путь к .json"), innerW))

	title := "Импорт правил из JSON"
	if m.jsonImportForm.asNew {
		title = "Новый профиль из JSON"
	}
	lines := append([]string{""}, strings.Split(renderBox(title, rows, w), "\n")...)
	lines = append(lines, "", clipLine(styleMuted.Render("  ↑↓ поля · Enter далее/импорт · Ctrl+S · Esc"), w))
	return padLines(lines, h)
}

func expandUserPath(p string) string {
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func parseExternalRulesJSON(raw string) ([]config.RoutingRule, error) {
	s := parser.NormalizeInput(raw)
	if s == "" {
		return nil, fmt.Errorf("вставь JSON или укажи путь к файлу")
	}

	var data []byte
	switch {
	case strings.HasPrefix(s, "[") || strings.HasPrefix(s, "{"):
		data = []byte(s)
	default:
		path := expandUserPath(s)
		b, err := os.ReadFile(path)
		if err != nil {
			// Путь мог прийти URL-кодированным — пробуем исходную строку.
			if alt := expandUserPath(parser.StripPasteMarkers(strings.TrimSpace(raw))); alt != path {
				if b2, err2 := os.ReadFile(alt); err2 == nil {
					b, err = b2, nil
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("не удалось прочитать файл: %w", err)
		}
		data = b
	}

	trimmed := strings.TrimSpace(string(data))
	var ext []externalRuleJSON

	switch {
	case strings.HasPrefix(trimmed, "["):
		if err := json.Unmarshal([]byte(trimmed), &ext); err != nil {
			return nil, fmt.Errorf("ошибка JSON-массива: %w", err)
		}
	case strings.HasPrefix(trimmed, "{"):
		var wrapper struct {
			Rules   []externalRuleJSON `json:"rules"`
			Routing struct {
				Rules []externalRuleJSON `json:"rules"`
			} `json:"routing"`
		}
		if err := json.Unmarshal([]byte(trimmed), &wrapper); err != nil {
			return nil, fmt.Errorf("ошибка JSON: %w", err)
		}
		if len(wrapper.Rules) > 0 {
			ext = wrapper.Rules
		} else {
			ext = wrapper.Routing.Rules
		}
	default:
		return nil, fmt.Errorf("ожидается JSON-массив [...] или объект с rules")
	}

	if len(ext) == 0 {
		return nil, fmt.Errorf("в JSON не найдено ни одного правила")
	}

	rules := make([]config.RoutingRule, 0, len(ext))
	for i, e := range ext {
		enabled := true
		if e.Enabled != nil {
			enabled = *e.Enabled
		}
		tag := strings.TrimSpace(e.OutboundTag)
		if tag == "" {
			tag = "proxy"
		}
		name := strings.TrimSpace(e.Remarks)
		if name == "" {
			name = fmt.Sprintf("Rule %d", i+1)
		}
		rules = append(rules, config.RoutingRule{
			ID:         config.NewID(),
			Name:       name,
			Enabled:    enabled,
			Outbound:   tag,
			Domains:    e.Domain,
			IPs:        e.IP,
			Port:       strings.TrimSpace(e.Port),
			Network:    strings.TrimSpace(e.Network),
			Protocol:   e.Protocol,
			Source:     e.Source,
			SourcePort: strings.TrimSpace(e.SourcePort),
			LocalIP:    e.LocalIP,
			LocalPort:  strings.TrimSpace(e.LocalPort),
			InboundTag: e.InboundTag,
			Attrs:      e.Attrs,
		})
	}
	return rules, nil
}

func exportProfileRulesJSON(rp config.RoutingProfile) (string, error) {
	out := make([]externalRuleJSON, 0, len(rp.Rules))
	for _, r := range rp.Rules {
		en := r.Enabled
		out = append(out, externalRuleJSON{
			Enabled:     &en,
			Locked:      false,
			OutboundTag: r.Outbound,
			Domain:      r.Domains,
			IP:          r.IPs,
			Port:        r.Port,
			Network:     r.Network,
			Protocol:    r.Protocol,
			Source:      r.Source,
			SourcePort:  r.SourcePort,
			LocalIP:     r.LocalIP,
			LocalPort:   r.LocalPort,
			InboundTag:  r.InboundTag,
			Attrs:       r.Attrs,
			Remarks:     r.Name,
		})
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}

	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cfgDir, "xray-tui", "exports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	safe := strings.NewReplacer("/", "_", "\\", "_", " ", "_").Replace(strings.TrimSpace(rp.Name))
	if safe == "" {
		safe = "profile"
	}
	path := filepath.Join(dir, safe+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func shortHomePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
