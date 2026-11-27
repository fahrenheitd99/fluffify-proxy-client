package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"xray-tui/internal/autostart"
	"xray-tui/internal/config"
	"xray-tui/internal/daemon"
	"xray-tui/internal/generator"
	"xray-tui/internal/ipc"
	"xray-tui/internal/osutil"
	"xray-tui/internal/parser"
	"xray-tui/internal/ping"
	"xray-tui/internal/uiutil"
)

type tabID int

const (
	tabServers tabID = iota
	tabSubs
	tabSettings
	tabLogs
)

const localGroupID = "__local__"

type pingState struct {
	loading bool
	done    bool
	subMs   bool
	ms      int64
	err     string
}

type treeRow struct {
	isHeader   bool
	groupID    string
	subIdx     int
	title      string
	count      int
	info       *config.SubInfo
	profileIdx int
}

type subFormState struct {
	active     bool
	mode       subFormMode // 0=подписка, 1=ссылка, 2=JSON
	step       int         // для подписки: 0=URL, 1=Название, 2=HWID
	url        string
	urlPasted  bool
	name       string
	sendHWID   bool
	link       string
	linkPasted bool
	rawJSON    string
	jsonPasted bool
}

// --- Сообщения Bubble Tea ---

type tickMsg time.Time

type animTickMsg struct {
	gen int
}

type statusMsg struct {
	status *ipc.StatusResponse
	err    error
}

type actionDoneMsg struct {
	status    *ipc.StatusResponse
	profileID string
	notice    string
	err       error
	// profile — профиль, который подключали (нужен для авто-починки TUN).
	profile *config.Profile
}

type pingResultMsg struct {
	profileID string
	rtt       time.Duration
	err       error
}

type subSyncDoneMsg struct {
	store  *config.Store
	notice string
	logs   []string
	err    error
}

type logLineMsg string

// elevateDoneMsg приходит после попытки перезапустить демон с правами админа.
type elevateDoneMsg struct{ err error }

type model struct {
	client *ipc.Client
	store  *config.Store

	width  int
	height int

	tab       tabID
	status    *ipc.StatusResponse
	netSpeed  netSpeedState
	daemonErr string
	notice    string
	noticeErr bool
	busy      bool

	// Анимация кнопки подключения
	connecting    bool
	animGen       int
	animTick      int
	revealStep    int
	hideStep      int
	pendingReveal bool

	// Вкладка Сервера
	cursor    int
	offset    int
	searching bool
	filter    string
	collapsed map[string]bool
	pings     map[string]pingState

	// Вкладка Подписки
	subCursor int
	subForm   subFormState
	burst     burstState

	// Сокет демона и профиль, который ждёт перезапуска демона с правами админа
	sock           string
	pendingConnect *config.Profile

	// Вкладка Настройки + профили роутинга
	settingsCursor    int
	settingsForm      settingsFormState
	autostartEnabled  bool
	routingView       routingScreen
	routingCursor     int
	routingEditIdx    int
	routingRuleCursor int
	routingProvCursor int
	ruleForm          ruleFormState
	profileForm       profileFormState
	providerForm      providerFormState
	jsonImportForm    jsonImportFormState

	// Вкладка Логи
	logs          []string
	logOffset     int
	logAutoScroll bool
	logCh         chan string

	// Мышь
	lastClickAt time.Time
	lastClickX  int
	lastClickY  int
}

func Run(socketPath string) error {
	store, err := config.Load()
	if err != nil {
		return err
	}
	applyTheme(store.Settings.Theme, store.Settings.AccentColor)

	client := ipc.NewClient(socketPath)
	logCh := make(chan string, 256)

	streamCtx, cancelStream := context.WithCancel(context.Background())
	defer cancelStream()

	go func() {
		for {
			_ = client.StreamLogs(streamCtx, func(line string) {
				select {
				case logCh <- line:
				default:
				}
			})
			select {
			case <-streamCtx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}()

	m := model{
		client:           client,
		store:            store,
		sock:             socketPath,
		width:            48,
		height:           34,
		collapsed:        make(map[string]bool),
		pings:            make(map[string]pingState),
		autostartEnabled: autostart.Enabled(),
		logAutoScroll:    true,
		logCh:            logCh,
	}

	rows := m.buildTreeRows()
	for i, r := range rows {
		if !r.isHeader && store.Profiles[r.profileIdx].ID == store.Settings.ActiveProfileID {
			m.cursor = i
			break
		}
	}
	m.clampTreeScroll()

	prog := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = prog.Run()
	return err
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchStatusCmd(),
		tickCmd(),
		waitForLogCmd(m.logCh),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		m = m.trackInputBurst(key)
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampTreeScroll()
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.fetchStatusCmd(), tickCmd())

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case animTickMsg:
		if msg.gen != m.animGen {
			return m, nil
		}
		if m.connecting {
			m.animTick++
			return m, animTickCmd(m.animGen)
		}
		if m.pendingReveal {
			m.animTick++
			if m.animTick >= 10 {
				m.pendingReveal = false
				m.revealStep = 1
			}
			return m, animTickCmd(m.animGen)
		}
		if m.revealStep > 0 {
			if m.revealStep < 8 {
				m.revealStep++
				return m, animTickCmd(m.animGen)
			}
			m.revealStep = 0
		}
		if m.hideStep > 0 {
			if m.hideStep < 8 {
				m.hideStep++
				return m, animTickCmd(m.animGen)
			}
			m.hideStep = 0
		}
		return m, nil

	case statusMsg:
		if msg.err != nil {
			m.daemonErr = msg.err.Error()
			m.status = nil
			m.netSpeed.reset()
		} else {
			m.daemonErr = ""
			m.status = msg.status
			if msg.status != nil && msg.status.Running {
				m.netSpeed.update(msg.status.Mode, msg.status.TUNName)
			} else {
				m.netSpeed.reset()
			}
		}
		return m, nil

	case actionDoneMsg:
		wasConnecting := m.connecting
		m.busy = false
		m.connecting = false
		if msg.err != nil {
			m.pendingReveal = false
			m.revealStep = 0
			m.hideStep = 0
			m.notice = msg.err.Error()
			m.noticeErr = true
		} else {
			m.daemonErr = ""
			if msg.status != nil {
				m.status = msg.status
			}
			if msg.profileID != "" {
				m.store.Settings.ActiveProfileID = msg.profileID
			}
			m.notice = msg.notice
			m.noticeErr = false
			if wasConnecting {
				m.hideStep = 0
				m.animGen++
				if m.animTick < 10 {
					m.pendingReveal = true
				} else {
					m.revealStep = 1
				}
				return m, tea.Batch(m.fetchStatusCmd(), animTickCmd(m.animGen))
			}
			m.pendingReveal = false
			m.revealStep = 0
		}
		return m, m.fetchStatusCmd()

	case pingResultMsg:
		if msg.err != nil {
			m.pings[msg.profileID] = pingState{done: true, err: "err"}
		} else {
			ms := msg.rtt.Milliseconds()
			subMs := msg.rtt < time.Millisecond
			if subMs {
				ms = 0
			}
			m.pings[msg.profileID] = pingState{done: true, subMs: subMs, ms: ms}
		}
		m.clampTreeScroll()
		return m, nil

	case subSyncDoneMsg:
		m.busy = false
		m.appendLogs(msg.logs)
		if msg.err != nil {
			m.notice = msg.err.Error()
			m.noticeErr = true
		} else {
			if msg.store != nil {
				m.store = msg.store
			}
			m.notice = msg.notice
			m.noticeErr = false
			m.clampTreeScroll()
			if m.subCursor >= len(m.store.Subscriptions) && len(m.store.Subscriptions) > 0 {
				m.subCursor = len(m.store.Subscriptions) - 1
			}
		}
		return m, nil

	case logLineMsg:
		m.appendLogs([]string{string(msg)})
		return m, waitForLogCmd(m.logCh)

	case geoDownloadDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.notice = "Ошибка геобаз: " + msg.err.Error()
			m.noticeErr = true
		} else {
			m.notice = fmt.Sprintf("Геобазы %s скачаны (%s)", msg.providerName, formatBytesShort(msg.status.TotalBytes))
			m.noticeErr = false
			// Если прокси уже работает с роутингом — пересобираем конфиг,
			// чтобы новые геобазы подхватились сразу.
			if m.store.Routing.Enabled {
				return m.applyRoutingNowKeep()
			}
		}
		return m, nil

	case elevateDoneMsg:
		if msg.err != nil {
			m.busy = false
			m.connecting = false
			m.pendingConnect = nil
			m.notice = "Не удалось получить права администратора: " + msg.err.Error()
			m.noticeErr = true
			return m, nil
		}
		if m.pendingConnect != nil {
			prof := *m.pendingConnect
			m.pendingConnect = nil
			return m.beginConnect(prof, "Демон с правами администратора — подключаюсь...")
		}
		m.busy = false
		m.notice = "Демон перезапущен с правами администратора"
		m.noticeErr = false
		return m, m.fetchStatusCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+c" {
		return m, tea.Quit
	}

	if m.subForm.active {
		return m.handleSubFormKey(msg)
	}

	if m.tab == tabSettings && m.inSettingsForm() {
		return m.handleSettingsFormKey(msg)
	}

	if m.tab == tabSettings && m.routingView != routingNone {
		if m.inRoutingForm() {
			return m.handleRoutingKey(msg)
		}
		if key == "q" {
			return m, tea.Quit
		}
		return m.handleRoutingKey(msg)
	}

	if m.searching {
		switch key {
		case "esc":
			m.searching = false
			m.filter = ""
			m.cursor = 0
			m.offset = 0
		case "enter":
			m.searching = false
		case "backspace":
			m.filter = trimLastRune(m.filter)
			m.cursor = 0
			m.offset = 0
		default:
			if add := extractInputText(msg); add != "" {
				m.filter += add
				m.cursor = 0
				m.offset = 0
			}
		}
		return m, nil
	}

	switch key {
	case "q":
		return m, tea.Quit
	case "1":
		m.tab = tabServers
		return m, nil
	case "2":
		m.tab = tabSubs
		return m, nil
	case "3":
		m.tab = tabSettings
		return m, nil
	case "4":
		m.tab = tabLogs
		if m.logAutoScroll {
			m.scrollLogsToBottom()
		}
		return m, nil
	case "5":
		return m, nil
	case "tab":
		m.tab = (m.tab + 1) % 4
		if m.tab == tabLogs && m.logAutoScroll {
			m.scrollLogsToBottom()
		}
		return m, nil
	case "shift+tab":
		m.tab = (m.tab + 3) % 4
		return m, nil
	}

	if key == " " && m.tab != tabSettings {
		return m.toggleConnection()
	}

	switch m.tab {
	case tabServers:
		return m.handleServersKey(key)
	case tabSubs:
		return m.handleSubsKey(key)
	case tabSettings:
		return m.handleSettingsKey(msg)
	case tabLogs:
		return m.handleLogsKey(key)
	}

	return m, nil
}

func (m model) handleServersKey(key string) (tea.Model, tea.Cmd) {
	rows := m.buildTreeRows()

	switch key {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.clampTreeScroll()
		}
	case "down", "j":
		if m.cursor < len(rows)-1 {
			m.cursor++
			m.clampTreeScroll()
		}
	case "ctrl+up", "[":
		m.jumpSubHeader(rows, -1)
	case "ctrl+down", "]":
		m.jumpSubHeader(rows, 1)
	case "g", "home":
		m.cursor = 0
		m.clampTreeScroll()
	case "G", "end":
		if len(rows) > 0 {
			m.cursor = len(rows) - 1
			m.clampTreeScroll()
		}
	case "left", "h":
		if len(rows) > 0 && m.cursor < len(rows) {
			r := rows[m.cursor]
			m.collapsed[r.groupID] = true
			if !r.isHeader {
				for i := m.cursor - 1; i >= 0; i-- {
					if rows[i].isHeader && rows[i].groupID == r.groupID {
						m.cursor = i
						break
					}
				}
			}
			m.clampTreeScroll()
		}
	case "right", "l":
		if len(rows) > 0 && m.cursor < len(rows) {
			r := rows[m.cursor]
			if r.isHeader {
				m.collapsed[r.groupID] = false
				m.clampTreeScroll()
			}
		}
	case "a":
		m.subForm = subFormState{active: true, sendHWID: true}
		return m, nil
	case "/":
		m.searching = true
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.cursor = 0
			m.offset = 0
		}
	case "d":
		if !m.busy && m.status != nil && m.status.Running {
			return m.beginDisconnect()
		}
	case "enter":
		if len(rows) > 0 && m.cursor < len(rows) {
			r := rows[m.cursor]
			if r.isHeader {
				m.collapsed[r.groupID] = !m.collapsed[r.groupID]
				m.clampTreeScroll()
				return m, nil
			}
			prof := m.store.Profiles[r.profileIdx]
			if m.status != nil && m.status.Running && m.status.Profile == prof.Name {
				return m.beginDisconnect()
			}
			return m.beginConnect(prof, "")
		}
	case "p":
		if len(rows) > 0 && m.cursor < len(rows) {
			r := rows[m.cursor]
			if r.isHeader {
				return m.pingGroup(r.groupID)
			}
			prof := m.store.Profiles[r.profileIdx]
			m.pings[prof.ID] = pingState{loading: true}
			return m, m.pingProfileCmd(prof, nil)
		}
	case "P":
		return m.pingAll()
	case "u":
		if !m.busy && len(rows) > 0 && m.cursor < len(rows) {
			r := rows[m.cursor]
			if r.subIdx >= 0 && r.subIdx < len(m.store.Subscriptions) {
				m.busy = true
				m.notice = "Обновляю: " + m.store.Subscriptions[r.subIdx].Name
				m.noticeErr = false
				return m, m.syncSubsCmd(r.subIdx)
			}
			m.notice = "Локальные профили не привязаны к подписке"
			m.noticeErr = false
		}
	case "U":
		if !m.busy && len(m.store.Subscriptions) > 0 {
			m.busy = true
			m.notice = "Обновляю все подписки..."
			m.noticeErr = false
			return m, m.syncSubsCmd(-1)
		}
	}
	return m, nil
}

func (m *model) jumpSubHeader(rows []treeRow, dir int) {
	if len(rows) == 0 {
		return
	}
	if dir > 0 {
		for i := m.cursor + 1; i < len(rows); i++ {
			if rows[i].isHeader {
				m.cursor = i
				m.clampTreeScroll()
				return
			}
		}
	} else {
		for i := m.cursor - 1; i >= 0; i-- {
			if rows[i].isHeader {
				m.cursor = i
				m.clampTreeScroll()
				return
			}
		}
	}
}

func (m model) pingGroup(groupID string) (tea.Model, tea.Cmd) {
	var targets []config.Profile
	for _, p := range m.store.Profiles {
		gid := p.SubID
		if gid == "" {
			gid = localGroupID
		}
		if gid == groupID {
			targets = append(targets, p)
		}
	}
	return m.runBatchPing(targets)
}

func (m model) pingAll() (tea.Model, tea.Cmd) {
	q := strings.ToLower(strings.TrimSpace(m.filter))
	var targets []config.Profile
	for _, p := range m.store.Profiles {
		if q == "" || m.matchesFilter(p, q) {
			targets = append(targets, p)
		}
	}
	return m.runBatchPing(targets)
}

func (m model) runBatchPing(targets []config.Profile) (tea.Model, tea.Cmd) {
	if len(targets) == 0 {
		return m, nil
	}
	workers := 4
	if m.store.Settings.PingMode == config.PingModeICMP {
		workers = 16

	}
	sem := make(chan struct{}, workers)
	cmds := make([]tea.Cmd, 0, len(targets))
	for _, prof := range targets {
		m.pings[prof.ID] = pingState{loading: true}
		cmds = append(cmds, m.pingProfileCmd(prof, sem))
	}
	m.notice = fmt.Sprintf("Пингую %d серверов (%s)...", len(targets), pingModeLabel(m.store.Settings.PingMode))
	m.noticeErr = false
	return m, tea.Batch(cmds...)
}

func (m model) handleSubsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.subCursor > 0 {
			m.subCursor--
		}
	case "down", "j":
		if m.subCursor < len(m.store.Subscriptions)-1 {
			m.subCursor++
		}
	case "a":
		m.subForm = subFormState{active: true, sendHWID: true}
		return m, nil
	case "enter", "u":
		if !m.busy && len(m.store.Subscriptions) > 0 && m.subCursor < len(m.store.Subscriptions) {
			m.busy = true
			m.notice = "Обновляю: " + m.store.Subscriptions[m.subCursor].Name
			m.noticeErr = false
			return m, m.syncSubsCmd(m.subCursor)
		}
	case "d":
		if !m.busy && len(m.store.Subscriptions) > 0 && m.subCursor < len(m.store.Subscriptions) {
			return m.deleteSubscriptionByIndex(m.subCursor)
		}
	case "U":
		if !m.busy && len(m.store.Subscriptions) > 0 {
			m.busy = true
			m.notice = "Обновляю все подписки..."
			m.noticeErr = false
			return m, m.syncSubsCmd(-1)
		}
	}
	return m, nil
}

func (m model) handleSubFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "esc" {
		m.subForm = subFormState{}
		return m, nil
	}

	// Переключение вкладок окна (Новая подписка / ссылка / JSON)
	switchTab := func(delta int) {
		m.subForm.mode = subFormMode((int(m.subForm.mode) + delta + 3) % 3)
		m.subForm.step = 0
	}
	if key == "tab" {
		switchTab(1)
		return m, nil
	}
	if key == "shift+tab" {
		switchTab(-1)
		return m, nil
	}
	if (key == "left" || key == "right") && !(m.subForm.mode == subModeSub && m.subForm.step == 2) {
		if key == "left" {
			switchTab(-1)
		} else {
			switchTab(1)
		}
		return m, nil
	}

	// Вкладка 2: одиночная ссылка (vless:// и др.)
	if m.subForm.mode == subModeLink {
		switch key {
		case "enter":
			return m.submitAddLink()
		case "backspace":
			m.subForm.link = trimLastRune(m.subForm.link)
		case " ":
			m.subForm.link += " "
		default:
			if add := extractInputText(msg); add != "" {
				m.subForm.link += add
			}
		}
		return m, nil
	}

	// Вкладка 3: голый JSON
	if m.subForm.mode == subModeJSON {
		switch key {
		case "enter":
			return m.submitAddJSON()
		case "backspace":
			m.subForm.rawJSON = trimLastRune(m.subForm.rawJSON)
		case " ":
			m.subForm.rawJSON += " "
		default:
			if add := extractInputText(msg); add != "" {
				m.subForm.rawJSON += add
			}
		}
		return m, nil
	}

	// Вкладка 1: Новая подписка
	switch key {
	case "up":
		if m.subForm.step > 0 {
			m.subForm.step--
		}
	case "down":
		if m.subForm.step < 2 {
			m.subForm.step++
		}
	case "left", "right", " ":
		if m.subForm.step == 2 {
			m.subForm.sendHWID = !m.subForm.sendHWID
		} else if key == " " && m.subForm.step == 1 {
			m.subForm.name += " "
		}
	case "enter":
		if m.subForm.step < 2 {
			if m.subForm.step == 0 && strings.TrimSpace(m.subForm.url) == "" {
				m.notice = "Введи URL подписки"
				m.noticeErr = true
				return m, nil
			}
			m.subForm.step++
			return m, nil
		}
		return m.applySmartInput(m.subForm.url)
	case "backspace":
		if m.subForm.step == 0 {
			m.subForm.url = trimLastRune(m.subForm.url)
		} else if m.subForm.step == 1 {
			m.subForm.name = trimLastRune(m.subForm.name)
		}
	default:
		if m.subForm.step == 0 || m.subForm.step == 1 {
			if add := extractInputText(msg); add != "" {
				if m.subForm.step == 0 {
					m.subForm.url += strings.TrimSpace(add)
				} else {
					m.subForm.name += add
				}
			}
		}
	}
	return m, nil
}

func (m model) renderSubForm(w, h int) string {
	innerW := maxInt(20, w-4)
	bar := styleAccent.Render("█")

	var rows []string
	var hint string

	switch m.subForm.mode {
	case subModeLink:
		val := fieldValue(m.subForm.link, m.subForm.linkPasted)
		if runes := []rune(val); len(runes) > 220 {
			val = "…" + string(runes[len(runes)-220:])
		}
		rows = append(rows, wrapField("Ключ (ссылка)", val+bar, true, innerW)...)
		hint = "  ←→ режим · Enter — добавить · Esc"

	case subModeJSON:
		val := fieldValue(m.subForm.rawJSON, m.subForm.jsonPasted)
		if runes := []rune(val); len(runes) > 220 {
			val = fmt.Sprintf("(%d симв.) …%s", len(runes), string(runes[len(runes)-200:]))
		}
		rows = append(rows, wrapField("JSON ноды", val+bar, true, innerW)...)
		hint = "  ←→ режим · Enter — добавить · Esc"

	default:
		uVal := fieldValue(m.subForm.url, m.subForm.urlPasted)
		nVal := sanitizeInput(m.subForm.name)
		if m.subForm.step == 0 {
			uVal += bar
		}
		if m.subForm.step == 1 {
			nVal += bar
		}
		rows = append(rows, wrapField("1. Ссылка или подписка", uVal, m.subForm.step == 0, innerW)...)
		rows = append(rows, wrapField("2. Имя (опц.)", nVal, m.subForm.step == 1, innerW)...)
		rows = append(rows, renderFormField("3. HWID (←/→)", "["+onOffLabel(m.subForm.sendHWID)+"]", m.subForm.step == 2, innerW))
		hint = "  ←→ режим · ↑↓ поля · Enter — далее / доб · Esc"
	}

	box := renderBox(" ", rows, w)
	boxLines := strings.Split(box, "\n")
	if len(boxLines) > 2 {
		hdr, _ := m.layoutSubFormHeader(innerW)
		pad := maxInt(0, innerW-termWidth(hdr))
		bdr := lipgloss.NewStyle().Foreground(colorBorder).Render("│")
		boxLines[1] = bdr + " " + hdr + strings.Repeat(" ", pad) + " " + bdr
	}

	lines := append([]string{""}, boxLines...)
	lines = append(lines, "", clipLine(styleMuted.Render(hint), w))
	return padLines(lines, h)
}

func (m model) handleLogsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.logAutoScroll = false
		if m.logOffset > 0 {
			m.logOffset--
		}
	case "down", "j":
		maxOff := m.maxLogOffset()
		if m.logOffset < maxOff {
			m.logOffset++
		}
		if m.logOffset >= maxOff {
			m.logAutoScroll = true
		}
	case "G", "end":
		m.logAutoScroll = true
		m.scrollLogsToBottom()
	case "g", "home":
		m.logAutoScroll = false
		m.logOffset = 0
	}
	return m, nil
}

func (m model) toggleConnection() (tea.Model, tea.Cmd) {
	if m.busy {
		return m, nil
	}
	if m.status != nil && m.status.Running {
		return m.beginDisconnect()
	}

	var prof *config.Profile
	rows := m.buildTreeRows()
	if m.tab == tabServers && len(rows) > 0 && m.cursor < len(rows) && !rows[m.cursor].isHeader {
		p := m.store.Profiles[rows[m.cursor].profileIdx]
		prof = &p
	} else if active := m.store.ActiveProfile(); active != nil {
		p := *active
		prof = &p
	} else if len(m.store.Profiles) > 0 {
		p := m.store.Profiles[0]
		prof = &p
	}

	if prof == nil {
		m.notice = "Нет серверов — нажми 'a' чтобы добавить подписку"
		m.noticeErr = true
		return m, nil
	}

	return m.beginConnect(*prof, "")
}

// --- Команды ---

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func animTickCmd(gen int) tea.Cmd {
	return tea.Tick(55*time.Millisecond, func(time.Time) tea.Msg {
		return animTickMsg{gen: gen}
	})
}

func (m model) beginConnect(prof config.Profile, notice string) (tea.Model, tea.Cmd) {
	// TUN в Windows возможен только у демона с правами администратора.
	// Поднимаем его через UAC, если точно знаем, что он не поднят или упал:
	// неизвестный статус не должен перезапускать работающий elevated-демон.
	if m.store.Settings.Mode == config.ModeTUN && osutil.TunRequiresAdmin() {
		daemonDown := m.daemonErr != "" || m.status == nil && m.daemonErr == ""
		if daemonDown || (m.status != nil && !m.status.Elevated) {
			pending := prof
			m.pendingConnect = &pending
			m.busy = true
			m.notice = "TUN: подтверди запрос прав администратора (UAC)"
			m.noticeErr = false
			return m, elevateDaemonCmd(m.sock)
		}
	}

	m.busy = true
	m.connecting = true
	m.animGen++
	m.animTick = 0
	m.revealStep = 0
	m.hideStep = 0
	m.pendingReveal = false
	m.notice = notice
	m.noticeErr = false
	return m, tea.Batch(m.connectProfileCmd(prof), animTickCmd(m.animGen))
}

func (m model) beginDisconnect() (tea.Model, tea.Cmd) {
	m.busy = true
	m.connecting = false
	m.pendingReveal = false
	m.revealStep = 0
	m.animGen++
	m.hideStep = 1
	m.notice = ""
	m.noticeErr = false
	return m, tea.Batch(m.stopCmd(), animTickCmd(m.animGen))
}

func waitForLogCmd(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		return logLineMsg(<-ch)
	}
}

func elevateDaemonCmd(sock string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		return elevateDoneMsg{err: daemon.RestartElevated(ctx, sock)}
	}
}

func (m model) fetchStatusCmd() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		st, err := client.Status()
		return statusMsg{status: st, err: err}
	}
}

func (m model) stopCmd() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		st, err := client.Stop()
		return actionDoneMsg{status: st, err: err}
	}
}

func (m model) connectProfileCmd(prof config.Profile) tea.Cmd {
	client := m.client
	storeCopy := *m.store
	mode := storeCopy.Settings.Mode
	tunName := storeCopy.Settings.TUNInterface

	return func() tea.Msg {
		res, err := generator.BuildForProfile(&storeCopy, &prof)
		if err != nil {
			return actionDoneMsg{err: err}
		}

		st, err := client.Start(ipc.StartRequest{
			Profile:   prof.Name,
			Mode:      mode,
			TUNName:   tunName,
			SocksAddr: res.SocksAddr,
			HTTPAddr:  res.HTTPAddr,
			Mixed:     res.Mixed,
			ProxyAuth: res.ProxyAuth,
			AssetDir:  res.AssetDir,
			Config:    json.RawMessage(res.Config),
		})
		if err != nil {
			return actionDoneMsg{err: err}
		}

		if freshStore, loadErr := config.Load(); loadErr == nil {
			freshStore.Settings.ActiveProfileID = prof.ID
			_ = freshStore.Save()
		}

		return actionDoneMsg{
			status:    st,
			profileID: prof.ID,
		}
	}
}

func (m model) pingProfileCmd(prof config.Profile, sem chan struct{}) tea.Cmd {
	pingMode := m.store.Settings.PingMode
	pingURL := m.store.Settings.PingURL
	opts := generator.DefaultOptions()
	opts.LogLevel = "error"

	bypassIface := ""
	if m.status != nil && m.status.Running && m.status.Mode == config.ModeTUN {
		tunName := m.status.TUNName
		if tunName == "" {
			tunName = m.store.Settings.TUNInterface
		}
		bypassIface = defaultRouteIface(tunName)
		if bypassIface == "" {
			bypassIface = ping.PhysicalIface(tunName)
		}
	}

	return func() tea.Msg {
		if sem != nil {
			sem <- struct{}{}
			defer func() { <-sem }()
		}

		if pingMode == config.PingModeICMP {
			addr := prof.Address
			if prof.Type == config.ProfileTypeRaw {
				if a, _, err := ping.ExtractEndpoint(prof.RawJSON); err == nil && a != "" {
					addr = a
				}
			}
			rtt, err := ping.MeasureICMP(context.Background(), addr, bypassIface)
			return pingResultMsg{profileID: prof.ID, rtt: rtt, err: err}
		}

		var cfg []byte
		var err error
		if prof.Type == config.ProfileTypeRaw {
			cfg = prof.RawJSON
		} else {
			node, parseErr := parser.ParseURI(prof.URI)
			if parseErr != nil {
				return pingResultMsg{profileID: prof.ID, err: parseErr}
			}
			cfg, err = generator.BuildFromNode(node, opts)
			if err != nil {
				return pingResultMsg{profileID: prof.ID, err: err}
			}
		}

		rtt, err := ping.Measure(context.Background(), osutil.CoreBinPath(), cfg, pingURL)
		return pingResultMsg{profileID: prof.ID, rtt: rtt, err: err}
	}
}

func (m model) syncSubsCmd(subIndex int) tea.Cmd {
	return func() tea.Msg {
		store, err := config.Load()
		if err != nil {
			return subSyncDoneMsg{err: err}
		}
		if len(store.Subscriptions) == 0 {
			return subSyncDoneMsg{err: fmt.Errorf("подписок пока нет")}
		}

		targets := make([]int, 0, len(store.Subscriptions))
		if subIndex >= 0 && subIndex < len(store.Subscriptions) {
			targets = append(targets, subIndex)
		} else {
			for i := range store.Subscriptions {
				targets = append(targets, i)
			}
		}

		var failed int
		var lastErr error
		var logs []string
		for _, idx := range targets {
			sub := &store.Subscriptions[idx]
			if err := syncOneSub(store, sub); err != nil {
				failed++
				lastErr = err
				logs = append(logs, fmt.Sprintf("[подписка] %s: %v", subLabel(sub), err))
				continue
			}
			count := 0
			for _, p := range store.Profiles {
				if p.SubID == sub.ID {
					count++
				}
			}
			logs = append(logs, fmt.Sprintf("[подписка] %s обновлена: %d нод", subLabel(sub), count))
		}
		if err := store.Save(); err != nil {
			return subSyncDoneMsg{err: err, logs: logs}
		}
		if failed > 0 && failed == len(targets) {
			return subSyncDoneMsg{store: store, err: lastErr, logs: logs}
		}
		msg := fmt.Sprintf("Обновлено (%d серверов)", len(store.Profiles))
		if len(targets) == 1 {
			msg = "Обновлена: " + store.Subscriptions[targets[0]].Name
		}
		if failed > 0 {
			msg = fmt.Sprintf("Обновлено с ошибками (%d)", failed)
		}
		return subSyncDoneMsg{store: store, notice: msg, logs: logs}
	}
}

// subLabel — человекочитаемое имя подписки для логов.
func subLabel(sub *config.Subscription) string {
	if strings.TrimSpace(sub.Name) != "" {
		return sub.Name
	}
	return sub.URL
}

// appendLogs добавляет строки в панель логи и не даёт ей расти бесконечно.
func (m *model) appendLogs(lines []string) {
	added := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		m.logs = append(m.logs, line)
		added = true
	}
	if !added {
		return
	}
	if len(m.logs) > 300 {
		m.logs = m.logs[len(m.logs)-300:]
	}
	if m.logAutoScroll {
		m.scrollLogsToBottom()
	}
}

func syncOneSub(store *config.Store, sub *config.Subscription) error {
	ua := sub.UserAgent
	if ua == "" {
		ua = store.Settings.DefaultUA
	}
	hwid := sub.CustomHWID
	if hwid == "" {
		hwid = store.Settings.HWID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	res, err := parser.FetchSubscription(ctx, sub.URL, parser.FetchOptions{
		UserAgent:   ua,
		SendHWID:    sub.SendHWID,
		HWID:        hwid,
		DeviceOS:    osutil.OSName(),
		DeviceVer:   config.DeviceOSVersion(),
		DeviceModel: config.DeviceModel(),
	})
	if err != nil {
		return err
	}

	if sub.Name == "" && res.Title != "" {
		sub.Name = res.Title
	}
	sub.UpdatedAt = time.Now()
	if res.UserInfo != nil {
		sub.Info = &config.SubInfo{
			Upload:   res.UserInfo.Upload,
			Download: res.UserInfo.Download,
			Total:    res.UserInfo.Total,
			Expire:   res.UserInfo.Expire,
		}
	}

	now := time.Now()
	profiles := make([]config.Profile, 0, len(res.Items))
	for _, it := range res.Items {
		profiles = append(profiles, config.Profile{
			ID:       config.NewID(),
			Name:     it.Name,
			Type:     it.Type,
			Protocol: it.Protocol,
			Address:  it.Address,
			Port:     it.Port,
			URI:      it.URI,
			RawJSON:  it.RawJSON,
			SubID:    sub.ID,
			AddedAt:  now,
		})
	}
	store.ReplaceSubProfiles(sub.ID, profiles)
	return nil
}

// --- Дерево серверов ---

func (m model) buildTreeRows() []treeRow {
	q := strings.ToLower(strings.TrimSpace(m.filter))
	rows := make([]treeRow, 0, len(m.store.Profiles)+len(m.store.Subscriptions)+1)

	appendGroup := func(groupID string, subIdx int, title string, info *config.SubInfo, indices []int) {
		if q != "" && len(indices) == 0 {
			return
		}
		if m.store.Settings.AutoSortPing {
			m.sortIndicesByPing(indices)
		}

		rows = append(rows, treeRow{
			isHeader:   true,
			groupID:    groupID,
			subIdx:     subIdx,
			title:      title,
			count:      len(indices),
			info:       info,
			profileIdx: -1,
		})

		if q == "" && m.collapsed[groupID] {
			return
		}
		for _, idx := range indices {
			rows = append(rows, treeRow{
				isHeader:   false,
				groupID:    groupID,
				subIdx:     subIdx,
				profileIdx: idx,
			})
		}
	}

	var localIndices []int
	for pIdx, p := range m.store.Profiles {
		if p.SubID == "" && (q == "" || m.matchesFilter(p, q)) {
			localIndices = append(localIndices, pIdx)
		}
	}
	if len(localIndices) > 0 {
		appendGroup(localGroupID, -1, "Ноды", nil, localIndices)
	}

	for sIdx, sub := range m.store.Subscriptions {
		var indices []int
		for pIdx, p := range m.store.Profiles {
			if p.SubID == sub.ID && (q == "" || m.matchesFilter(p, q)) {
				indices = append(indices, pIdx)
			}
		}
		appendGroup(sub.ID, sIdx, sub.Name, sub.Info, indices)
	}

	return rows
}

func (m model) matchesFilter(p config.Profile, q string) bool {
	return strings.Contains(strings.ToLower(p.Name), q) ||
		strings.Contains(strings.ToLower(p.Protocol), q) ||
		strings.Contains(strings.ToLower(m.store.SubName(p.SubID)), q)
}

func (m model) sortIndicesByPing(indices []int) {
	rank := func(idx int) (int, int64) {
		ps, ok := m.pings[m.store.Profiles[idx].ID]
		if !ok || ps.loading || !ps.done {
			return 1, 0
		}
		if ps.err != "" {
			return 2, 0
		}
		return 0, ps.ms
	}

	sort.SliceStable(indices, func(i, j int) bool {
		ri, mi := rank(indices[i])
		rj, mj := rank(indices[j])
		if ri != rj {
			return ri < rj
		}
		if ri == 0 {
			return mi < mj
		}
		return false
	})
}

// --- Отрисовка ---

func (m model) View() string {
	w := m.width
	if w < 38 {
		w = 38
	}

	var b strings.Builder

	b.WriteString(m.renderBreadcrumbs(w))
	b.WriteString("\n")
	b.WriteString(m.renderHeader(w))
	b.WriteString("\n")
	b.WriteString(m.renderTabs(w))
	b.WriteString("\n")

	if m.tab == tabServers && (m.searching || m.filter != "") {
		prompt := styleAccent.Render("/") + " " + m.filter
		if m.searching {
			prompt += styleAccent.Render("█")
		} else {
			prompt += styleMuted.Render("  (esc — сброс)")
		}
		b.WriteString(clipLine(prompt, w))
		b.WriteString("\n")
	}

	bodyHeight := m.listHeight()
	if m.subForm.active {
		b.WriteString(m.renderSubForm(w, bodyHeight))
	} else {
		switch m.tab {
		case tabServers:
			b.WriteString(m.renderServers(w, bodyHeight))
		case tabSubs:
			b.WriteString(m.renderSubs(w, bodyHeight))
		case tabSettings:
			if m.routingView != routingNone {
				b.WriteString(m.renderRoutingScreen(w, bodyHeight))
			} else {
				b.WriteString(m.renderSettings(w, bodyHeight))
			}
		case tabLogs:
			b.WriteString(m.renderLogs(w, bodyHeight))
		}
	}

	b.WriteString("\n")
	b.WriteString(m.renderFooter(w))

	return fitView(b.String(), m.width, m.height)
}

func (m model) headerTitleRaw() string {
	switch {
	case m.daemonErr != "":
		return "Нет связи с демоном"
	case m.status != nil && m.status.Running:
		if m.status.Profile != "" {
			return m.status.Profile
		}
		return "active"
	default:
		if active := m.store.ActiveProfile(); active != nil {
			return active.Name
		}
		return "Отключено"
	}
}

func (m model) headerNameLines(w int) []string {
	innerW := maxInt(20, w-4)
	nameMaxW := maxInt(10, innerW-statusButtonW-1)
	lines := wrapTextByWidth(uiutil.Display(m.headerTitleRaw()), nameMaxW)
	if len(lines) == 0 {
		return []string{""}
	}
	if len(lines) > 3 {
		lines = lines[:3]
		lines[2] = clipLine(lines[2], nameMaxW)
	}
	return lines
}

func (m model) renderHeader(w int) string {
	innerW := maxInt(20, w-4)
	nameMaxW := maxInt(10, innerW-statusButtonW-1)

	var state, line2, line3 string
	var titleStyle lipgloss.Style

	switch {
	case m.daemonErr != "":
		state = "err"
		titleStyle = styleRed
		line2 = styleMuted.Render(daemonStartHint())
		line3 = styleMuted.Render(m.daemonErr)

	case m.status != nil && m.status.Running:
		state = "on"
		titleStyle = styleTitle

		modeTag := "PROXY"
		var endpoint string
		if m.status.Mode == config.ModeTUN {
			modeTag = "TUN"
			iface := m.status.TUNName
			if iface == "" {
				iface = m.store.Settings.TUNInterface
			}
			endpoint = iface
		} else if m.status.Mixed {
			endpoint = "MIXED " + m.status.SocksAddr
		} else if m.status.HTTPAddr != "" {
			_, sp, _ := strings.Cut(m.status.SocksAddr, ":")
			_, hp, _ := strings.Cut(m.status.HTTPAddr, ":")
			endpoint = fmt.Sprintf("SOCKS:%s HTTP:%s", sp, hp)
		} else {
			endpoint = "SOCKS5 " + m.status.SocksAddr
		}
		if m.status.ProxyAuth {
			endpoint += " 🔒"
		}
		line2 = styleMuted.Render(fmt.Sprintf("%s · %s", modeTag, endpoint))

		uptime := formatDuration(time.Duration(m.status.UptimeSec) * time.Second)
		meta := uptime
		if active := m.store.ActiveProfile(); active != nil {
			if ps, ok := m.pings[active.ID]; ok && ps.done && ps.err == "" {
				meta += " · " + formatPing(ps)
			}
		}
		line3 = styleMuted.Render(meta)

	default:
		state = "off"
		titleStyle = styleMuted

		modeTag := "PROXY"
		if m.store.Settings.Mode == config.ModeTUN {
			modeTag = "TUN"
		}
		line2 = styleMuted.Render(modeTag + " · отключено")
		line3 = styleMuted.Render("Enter или Space")
	}

	waving := m.connecting || m.pendingReveal
	btn := renderStatusButton(state, waving, m.animTick, m.revealStep, m.hideStep)

	nameLines := m.headerNameLines(w)
	var botRow1, botRow2 string

	if state == "err" {
		botRow1 = clipLine(line2, nameMaxW)
		botRow2 = clipLine(line3, nameMaxW)
	} else {
		// Скорость подключения (↓/↑ B/s) из шапки убрана — слева только
		// статусные строки, справа иконка подключения.
		botRow1 = clipLine(line2, nameMaxW)
		botRow2 = clipLine(line3, nameMaxW)
	}

	leftCols := make([]string, 0, len(nameLines)+2)
	for _, raw := range nameLines {
		leftCols = append(leftCols, titleStyle.Render(clipLine(raw, nameMaxW)))
	}
	leftCols = append(leftCols, botRow1, botRow2)

	totalRows := len(leftCols)
	btnTop := (totalRows - statusButtonH) / 2
	emptyBtn := strings.Repeat(" ", statusButtonW)

	cardLines := make([]string, totalRows)
	for i := 0; i < totalRows; i++ {
		l := leftCols[i]
		lw := termWidth(l)
		if lw > nameMaxW {
			l = clipLine(l, nameMaxW)
			lw = termWidth(l)
		}
		rightPart := emptyBtn
		if bIdx := i - btnTop; bIdx >= 0 && bIdx < statusButtonH {
			rightPart = btn[bIdx]
		}
		pad := maxInt(1, innerW-lw-statusButtonW)
		cardLines[i] = l + strings.Repeat(" ", pad) + rightPart
	}

	cardState := state
	if state == "on" && (waving || (m.revealStep > 0 && m.revealStep < 4)) {
		cardState = "off"
	} else if state == "off" && m.hideStep > 0 && m.hideStep < 4 {
		cardState = "on"
	}
	return renderHeaderCard(cardLines, w, cardState)
}

func wrapTextByWidth(s string, maxW int) []string {
	s = strings.TrimSpace(s)
	if s == "" || maxW <= 0 {
		return []string{""}
	}
	if termWidth(s) <= maxW {
		return []string{s}
	}

	words := strings.Fields(s)
	var lines []string
	cur := ""

	for _, w := range words {
		if cur == "" {
			if termWidth(w) <= maxW {
				cur = w
			} else {
				chunks := splitTokenByWidth(w, maxW)
				lines = append(lines, chunks[:len(chunks)-1]...)
				cur = chunks[len(chunks)-1]
			}
			continue
		}
		cand := cur + " " + w
		if termWidth(cand) <= maxW {
			cur = cand
		} else {
			lines = append(lines, cur)
			if termWidth(w) <= maxW {
				cur = w
			} else {
				chunks := splitTokenByWidth(w, maxW)
				lines = append(lines, chunks[:len(chunks)-1]...)
				cur = chunks[len(chunks)-1]
			}
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

func splitTokenByWidth(s string, maxW int) []string {
	runes := []rune(s)
	var clusters []string
	for i := 0; i < len(runes); {
		j := i + 1
		if runes[i] >= 0x1F1E6 && runes[i] <= 0x1F1FF && j < len(runes) && runes[j] >= 0x1F1E6 && runes[j] <= 0x1F1FF {
			j++
		}
		for j < len(runes) {
			if runes[j] == 0xFE0F || runes[j] == 0xFE0E {
				j++
			} else if runes[j] == 0x200D && j+1 < len(runes) {
				j += 2
			} else {
				break
			}
		}
		clusters = append(clusters, string(runes[i:j]))
		i = j
	}

	var out []string
	cur := ""
	for _, cl := range clusters {
		if cur != "" && termWidth(cur+cl) > maxW {
			out = append(out, cur)
			cur = cl
		} else {
			cur += cl
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func (m model) renderTabs(w int) string {
	var labels [4]string
	if w < 58 {
		labels = [4]string{
			fmt.Sprintf("1:Серв(%d)", len(m.store.Profiles)),
			fmt.Sprintf("2:Подп(%d)", len(m.store.Subscriptions)),
			"3:Настр",
			"4:Логи",
		}
	} else {
		labels = [4]string{
			fmt.Sprintf("[1] Сервера (%d)", len(m.store.Profiles)),
			fmt.Sprintf("[2] Подписки (%d)", len(m.store.Subscriptions)),
			"[3] Настройки",
			"[4] Логи",
		}
	}

	parts := make([]string, 4)
	for i, lbl := range labels {
		if tabID(i) == m.tab {
			parts[i] = renderTabActive(lbl)
		} else {
			parts[i] = styleTabIdle.Render(lbl)
		}
	}

	sep := "  "
	if w < 52 {
		sep = " "
	}
	return clipLine(" "+strings.Join(parts, sep), w)
}

func (m model) renderServers(w, h int) string {
	rows := m.buildTreeRows()
	if len(rows) == 0 {
		msg := "  Серверов нет. Нажми 'a' чтобы добавить подписку."
		if m.filter != "" {
			msg = fmt.Sprintf("  Ничего не найдено по %q", m.filter)
		}
		return padLines([]string{"", styleMuted.Render(msg)}, h)
	}

	isRunning := m.status != nil && m.status.Running
	activeName := ""
	if isRunning {
		activeName = uiutil.Display(m.status.Profile)
	}

	lines := make([]string, 0, h)
	end := m.offset + h
	if end > len(rows) {
		end = len(rows)
	}

	for pos := m.offset; pos < end; pos++ {
		r := rows[pos]
		selected := pos == m.cursor

		if r.isHeader {
			arrow := " ▾  "
			if m.filter == "" && m.collapsed[r.groupID] {
				arrow = " ▸  "
			}
			prefix := " "
			if selected {
				prefix = styleAccent.Render("┃")
			}

			rightMeta := fmt.Sprintf("[%d]", r.count)
			if r.info != nil {
				rightMeta = fmt.Sprintf("%s · [%d]", formatSubCompact(r.info), r.count)
			}
			rightRendered := styleMuted.Render(" " + rightMeta + " ")

			leftFixed := termWidth(prefix) + termWidth(arrow)
			rightW := termWidth(rightRendered)
			titleMax := maxInt(8, w-leftFixed-rightW-3)

			titleText := clipLine(uiutil.Display(r.title), titleMax)
			if selected {
				titleText = styleCursorRow.Bold(true).Render(titleText)
				arrow = styleAccent.Bold(true).Render(arrow)
			} else {
				titleText = styleGroupRow.Bold(true).Render(titleText)
				arrow = styleAccent.Render(arrow)
			}

			fillW := maxInt(1, w-leftFixed-termWidth(titleText)-rightW)
			var midFill string
			if fillW >= 4 {
				midFill = " " + styleMuted.Render(strings.Repeat("─", fillW-2)) + " "
			} else {
				midFill = strings.Repeat(" ", fillW)
			}

			headerLine := clipLine(prefix+arrow+titleText+midFill+rightRendered, w)
			if selected {
				headerLine = highlightRow(headerLine, w)
			}
			lines = append(lines, headerLine)
			continue
		}

		p := m.store.Profiles[r.profileIdx]
		prefix := "   "
		if selected {
			prefix = " " + styleAccent.Render("▸ ")
		}

		dot := "  "
		if isRunning && p.Name == activeName {
			dot = styleGreen.Render("● ")
		} else if p.ID == m.store.Settings.ActiveProfileID {
			dot = styleMuted.Render("○ ")
		}

		protoTag := styleMuted.Render(fmt.Sprintf("[%-5s] ", shortProto(p.Protocol)))

		right := ""
		if ps, ok := m.pings[p.ID]; ok {
			right = " " + formatPing(ps)
		}

		leftFixed := termWidth(prefix) + termWidth(dot) + termWidth(protoTag)
		rightW := termWidth(right)
		nameMax := maxInt(8, w-leftFixed-rightW-1)

		nameText := clipLine(uiutil.Display(p.Name), nameMax)
		if selected {
			nameText = styleCursorRow.Render(nameText)
		}

		pad := maxInt(0, w-leftFixed-termWidth(nameText)-rightW)
		serverLine := clipLine(prefix+dot+protoTag+nameText+strings.Repeat(" ", pad)+right, w)
		if selected {
			serverLine = highlightRow(serverLine, w)
		}
		lines = append(lines, serverLine)
	}

	return padLines(lines, h)
}

func (m model) renderSubs(w, h int) string {
	if len(m.store.Subscriptions) == 0 {
		return padLines([]string{"", styleMuted.Render("  Подписок пока нет. Нажми 'a' чтобы добавить.")}, h)
	}

	var allLines []string
	cursorStartLine := 0

	for i, sub := range m.store.Subscriptions {
		if i == m.subCursor {
			cursorStartLine = len(allLines)
		}

		count := 0
		for _, p := range m.store.Profiles {
			if p.SubID == sub.ID {
				count++
			}
		}

		prefix := "  "
		nameRendered := styleTitle.Render(sub.Name)
		if i == m.subCursor {
			prefix = styleAccent.Render("▸ ")
			nameRendered = styleCursorRow.Render(sub.Name)
		}

		ua := sub.UserAgent
		if ua == "" {
			ua = m.store.Settings.DefaultUA + " (global)"
		}

		hwidStr := "выкл"
		if sub.SendHWID {
			if sub.CustomHWID != "" {
				hwidStr = sub.CustomHWID + " (custom)"
			} else {
				hwidStr = m.store.Settings.HWID + " (global)"
			}
		}

		updatedStr := "никогда"
		if !sub.UpdatedAt.IsZero() {
			updatedStr = sub.UpdatedAt.Format("02.01.2006 15:04")
		}

		titleLine := clipLine(fmt.Sprintf("%s%s %s", prefix, nameRendered, styleMuted.Render(fmt.Sprintf("(%d серв.)", count))), w)
		if i == m.subCursor {
			titleLine = highlightRow(titleLine, w)
		}

		allLines = append(allLines,
			titleLine,
			clipLine(styleMuted.Render("   URL:  ")+sub.URL, w),
			clipLine(styleMuted.Render("   UA:   ")+ua, w),
			clipLine(styleMuted.Render("   HWID: ")+hwidStr, w),
		)

		if sub.Info != nil {
			allLines = append(allLines, clipLine(styleMuted.Render("   Траф: ")+formatSubInfo(sub.Info), w))
		}
		allLines = append(allLines,
			clipLine(styleMuted.Render("   Обн:  "+updatedStr), w),
			"",
		)
	}

	start := 0
	if cursorStartLine+6 > h {
		start = cursorStartLine - 2
	}
	if start < 0 {
		start = 0
	}
	if start > len(allLines) {
		start = len(allLines)
	}
	end := start + h
	if end > len(allLines) {
		end = len(allLines)
	}

	return padLines(allLines[start:end], h)
}

func renderBox(title string, rows []string, totalW int) string {
	return renderSectionCard(title, rows, totalW)
}

func renderSettingRow(label, value string, selected bool, innerW int) string {
	prefix := "  "
	lbl := label
	val := styleMuted.Render("[" + value + "]")
	if selected {
		prefix = styleAccent.Render("▸ ")
		lbl = styleCursorRow.Render(label)
		val = styleAccent.Render("[" + value + "]")
	}
	gap := innerW - termWidth(prefix) - termWidth(lbl) - termWidth(val)
	if gap < 1 {
		gap = 1
	}
	row := clipLine(prefix+lbl+strings.Repeat(" ", gap)+val, innerW)
	if selected {
		return highlightRow(row, innerW)
	}
	return row
}

func (m model) wrappedLogLines(w int) []string {
	contentW := maxInt(10, w-2)
	out := make([]string, 0, len(m.logs))
	for _, raw := range m.logs {
		style := styleMuted
		switch {
		case strings.Contains(raw, "[Error]") || strings.Contains(raw, "Failed to start") || strings.Contains(raw, "ошибк"):
			style = styleRed
		case strings.Contains(raw, "[Warning]"):
			style = styleYellow
		}
		chunks := wrapTerm(uiutil.Display(raw), contentW)
		if len(chunks) == 0 {
			out = append(out, "")
			continue
		}
		for _, ch := range chunks {
			out = append(out, style.Render(" "+ch))
		}
	}
	return out
}

func (m model) renderLogs(w, h int) string {
	if len(m.logs) == 0 {
		return padLines([]string{"", styleMuted.Render("  Логов пока нет.")}, h)
	}

	wrapped := m.wrappedLogLines(w)
	start := m.logOffset
	if start < 0 {
		start = 0
	}
	maxOff := len(wrapped) - h
	if maxOff < 0 {
		maxOff = 0
	}
	if start > maxOff {
		start = maxOff
	}
	end := start + h
	if end > len(wrapped) {
		end = len(wrapped)
	}
	return padLines(wrapped[start:end], h)
}

func (m model) renderFooter(w int) string {
	sep := renderSeparator(w)

	var help string
	narrow := w < 60
	switch m.tab {
	case tabServers:
		if narrow {
			help = "⏎ вкл/откл · a подп · p/P · u · d · q"
		} else {
			help = "enter вкл/откл · d откл · a подписка · ctrl+↑↓ · p/P пинг · u обн · / поиск · q"
		}
	case tabSubs:
		if narrow {
			help = "a доб · ⏎/u обн · U все · spc · q выход"
		} else {
			help = "a добавить · enter/u обновить · U обновить все · space тоггл · q выход"
		}
	case tabSettings:
		switch m.routingView {
		case routingProfiles:
			help = "⏎ откр · spc выбр · a нов · i имп · e эксп · d уд · r"
		case routingProfileEdit:
			help = "⏎ прав · spc вкл · ⇧↑↓ · a · i имп · E эксп · d · r"
		case routingProviders:
			help = "⏎/u скачать · a добавить · d удалить · esc назад"
		default:
			help = "↑↓ выбор · ⏎/spc перекл · e свой ввод · q"
		}
	case tabLogs:
		mode := "авто"
		if !m.logAutoScroll {
			mode = "пауза (G)"
		}
		if narrow {
			help = fmt.Sprintf("↑↓ скролл · %s · spc · q выход", mode)
		} else {
			help = fmt.Sprintf("↑↓ скролл · %s · space тоггл · q выход", mode)
		}
	}

	var bottomLine string
	if m.store.Settings.UIMode == config.UIModeButtons {
		bottomLine, _ = m.layoutFooterButtons(w)
	} else {
		bottomLine = clipLine(" "+styleMuted.Render(help), w)
	}

	if m.notice != "" {
		st := styleAccent
		if m.noticeErr {
			st = styleRed
		}
		return sep + "\n" + clipLine(" "+st.Render(uiutil.Display(m.notice)), w) + "\n" + bottomLine
	}
	return sep + "\n" + bottomLine
}

// --- Вспомогательные функции ---

func (m model) listHeight() int {
	w := m.width
	if w < 38 {
		w = 38
	}
	headerRows := len(m.headerNameLines(w)) + 2
	reserved := headerRows + 5 + m.breadcrumbHeight(w) // +2 рамка шапки +1 вкладки +2 футер + путь
	if m.notice != "" {
		reserved++
	}
	if m.tab == tabServers && (m.searching || m.filter != "") {
		reserved++
	}
	if m.store.Settings.UIMode == config.UIModeButtons {
		reserved += 2
	}
	h := m.height - reserved
	if h < 4 {
		h = 4
	}
	return h
}

func (m *model) clampTreeScroll() {
	n := len(m.buildTreeRows())
	if n == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m model) maxLogOffset() int {
	w := m.width
	if w < 38 {
		w = 38
	}
	maxOff := len(m.wrappedLogLines(w)) - m.listHeight()
	if maxOff < 0 {
		return 0
	}
	return maxOff
}

func (m *model) scrollLogsToBottom() {
	m.logOffset = m.maxLogOffset()
}

func modeLabel(mode string) string {
	if mode == config.ModeTUN {
		return "TUN"
	}
	return "Proxy"
}

func pingModeLabel(mode string) string {
	if mode == config.PingModeICMP {
		return "ICMP Ping"
	}
	return "via proxy HEAD"
}
func shortProto(p string) string {
	p = strings.TrimPrefix(p, "raw/")
	switch p {
	case "hysteria", "hysteria2":
		return "hy2"
	case "shadowsocks":
		return "ss"
	case "wireguard":
		return "wg"
	default:
		if len(p) > 5 {
			return p[:5]
		}
		return p
	}
}

func formatPing(ps pingState) string {
	if ps.loading {
		return styleMuted.Render("...")
	}
	if ps.err != "" {
		return stylePingErr.Render(" err ")
	}
	txt := fmt.Sprintf("%d ms", ps.ms)
	if ps.subMs {
		txt = "<1 ms"
	}
	switch {
	case ps.ms < 500:
		return styleGreen.Render(txt)
	case ps.ms < 1000:
		return styleYellow.Render(txt)
	default:
		return styleRed.Render(txt)
	}
}

func formatDuration(d time.Duration) string {
	sec := int64(d.Seconds())
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm %02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func formatSubCompact(info *config.SubInfo) string {
	used := info.Upload + info.Download
	if info.Total > 0 {
		return fmt.Sprintf("%s/%s", formatBytesShort(used), formatBytesShort(info.Total))
	}
	return formatBytesShort(used)
}

func formatSubInfo(info *config.SubInfo) string {
	used := info.Upload + info.Download
	out := formatBytes(used)
	if info.Total > 0 {
		out += " / " + formatBytes(info.Total)
	}
	if info.Expire > 0 {
		out += " · до " + time.Unix(info.Expire, 0).Format("2006-01-02")
	}
	return out
}

func formatBytesShort(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%c", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func clipLine(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	// Обрезаем по клеткам терминала (эмодзи = 2 клетки), а не по таблицам
	// lipgloss: иначе строка, «влезающая» по lipgloss.Width, в реальном
	// терминале окажется шире экрана и перенесётся на вторую строку.
	return truncateTerm(s, maxW)
}

// fitView приводит кадр к ровно h строкам шириной не более w.
// Кадр, выходящий за пределы терминала (или строка шире экрана, которую
// терминал переносит на вторую строку), заставляет altscreen скроллиться —
// рендерер не может вернуть курсор выше верха экрана, и вся отрисовка
// начинает «плыть» вверх при прокрутке.
func fitView(content string, w, h int) string {
	if w < 1 || h < 1 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	for i, line := range lines {
		lines[i] = clipLine(line, w)
	}
	return applyCanvas(strings.Join(lines, "\n"), w, h)
}

func padLines(lines []string, h int) string {
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}
