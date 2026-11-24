package tray

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"fyne.io/systray"

	"xray-tui/internal/config"
	"xray-tui/internal/generator"
	"xray-tui/internal/ipc"
)

func lockFilePath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "fluffify-tray.lock")
}

func acquireTrayLock() (*os.File, bool) {
	f, err := os.OpenFile(lockFilePath(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, false
	}
	return f, true
}

// EnsureRunning запускает иконку трея в отвязанной фоновой сессии, если она ещё не запущена.
func EnsureRunning() {
	if f, ok := acquireTrayLock(); !ok {
		return // трей уже работает
	} else {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}

	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "tray")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	_ = cmd.Start()
}

func Run(socketPath string) {
	lock, ok := acquireTrayLock()
	if !ok {
		return
	}
	defer func() {
		_ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
		_ = lock.Close()
	}()

	systray.Run(func() {
		onReady(socketPath)
	}, func() {})
}

type nodeMenuEntry struct {
	profile config.Profile
	item    *systray.MenuItem
	checked bool
}

type routingMenuEntry struct {
	id      string
	item    *systray.MenuItem
	checked bool
}

const maxMenuChunk = 9999

func onReady(socketPath string) {
	client := ipc.NewClient(socketPath)

	kdeGlobalsPath := filepath.Join(userHomeDir(), ".config", "kdeglobals")
	lastKdeMod := fileModTime(kdeGlobalsPath)
	accentHex := readKDEAccentColor(kdeGlobalsPath)
	iconOff, iconOn, iconErr := buildTrayIcons(accentHex)

	systray.SetIcon(iconOff)
	systray.SetTitle("Fluffify")
	systray.SetTooltip("Fluffify — отключено")

	mStatus := systray.AddMenuItem("Отключено", "Текущий статус")
	mStatus.Disable()

	systray.AddSeparator()
	mToggle := systray.AddMenuItem("Подключить", "Включить или выключить VPN")
	mOpen := systray.AddMenuItem("Открыть Fluffify", "Открыть TUI в терминале")

	initStore, _ := config.Load()

	// Подменю выбора серверов (сгруппировано по подпискам)
	mServers := systray.AddMenuItem("Серверы", "Выбрать и подключить ноду")
	selectProfCh := make(chan config.Profile, 4)
	var nodeItems []nodeMenuEntry

	addNodeItem := func(parent *systray.MenuItem, prof config.Profile) {
		mi := parent.AddSubMenuItemCheckbox(prof.Name, "", false)
		nodeItems = append(nodeItems, nodeMenuEntry{profile: prof, item: mi})
		go func(p config.Profile, item *systray.MenuItem) {
			for range item.ClickedCh {
				selectProfCh <- p
			}
		}(prof, mi)
	}

	if initStore != nil && len(initStore.Profiles) > 0 {
		type group struct {
			title    string
			profiles []config.Profile
		}
		var groups []group

		var local []config.Profile
		for _, p := range initStore.Profiles {
			if p.SubID == "" {
				local = append(local, p)
			}
		}
		if len(local) > 0 {
			groups = append(groups, group{title: "Ноды", profiles: local})
		}

		for _, sub := range initStore.Subscriptions {
			var list []config.Profile
			for _, p := range initStore.Profiles {
				if p.SubID == sub.ID {
					list = append(list, p)
				}
			}
			if len(list) > 0 {
				groups = append(groups, group{title: sub.Name, profiles: list})
			}
		}

		for _, g := range groups {
			parent := mServers
			if len(groups) > 1 {
				parent = mServers.AddSubMenuItem(fmt.Sprintf("%s (%d)", g.title, len(g.profiles)), "")
			}

			if len(g.profiles) <= maxMenuChunk {
				for _, prof := range g.profiles {
					addNodeItem(parent, prof)
				}
			} else {
				for start := 0; start < len(g.profiles); start += maxMenuChunk {
					end := start + maxMenuChunk
					if end > len(g.profiles) {
						end = len(g.profiles)
					}
					chunkMenu := parent.AddSubMenuItem(fmt.Sprintf("%d–%d", start+1, end), "")
					for _, prof := range g.profiles[start:end] {
						addNodeItem(chunkMenu, prof)
					}
				}
			}
		}
	} else {
		empty := mServers.AddSubMenuItem("Нет серверов", "")
		empty.Disable()
	}

	// Подменю выбора профиля правил роутинга
	mRouting := systray.AddMenuItem("Роутинг", "Выбрать профиль правил роутинга")
	selectRoutingCh := make(chan string, 4) // "" = выключить роутинг
	mRoutingOff := mRouting.AddSubMenuItemCheckbox("Выкл (без правил)", "", false)
	routingOffChecked := false
	go func() {
		for range mRoutingOff.ClickedCh {
			selectRoutingCh <- ""
		}
	}()

	var routingItems []routingMenuEntry
	if initStore != nil {
		for _, rp := range initStore.Routing.Profiles {
			mi := mRouting.AddSubMenuItemCheckbox(rp.Name, "", false)
			routingItems = append(routingItems, routingMenuEntry{id: rp.ID, item: mi})
			go func(id string, item *systray.MenuItem) {
				for range item.ClickedCh {
					selectRoutingCh <- id
				}
			}(rp.ID, mi)
		}
	}

	systray.AddSeparator()
	mMode := systray.AddMenuItem("Режим", "Переключить TUN / Proxy")
	mModeTUN := mMode.AddSubMenuItemCheckbox("TUN", "Системный TUN", false)
	mModeProxy := mMode.AddSubMenuItemCheckbox("Proxy", "Локальный SOCKS/HTTP", false)

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Выйти", "Остановить демон, Xray и закрыть Fluffify")

	// Кэш состояния — шлём обновления в DBus только при реальных изменениях
	lastIconState := "off"
	lastTooltip := "Fluffify — отключено"
	lastStatusTitle := "Отключено"
	lastToggleTitle := "Подключить"
	modeTUNChecked := false
	modeProxyChecked := false

	setCheckbox := func(item *systray.MenuItem, cur *bool, want bool) {
		if *cur == want {
			return
		}
		*cur = want
		if want {
			item.Check()
		} else {
			item.Uncheck()
		}
	}

	syncUI := func() {
		// Живое обновление цвета при смене акцента в настройках KDE Plasma
		if mod := fileModTime(kdeGlobalsPath); !mod.Equal(lastKdeMod) {
			lastKdeMod = mod
			if curAccent := readKDEAccentColor(kdeGlobalsPath); curAccent != accentHex {
				accentHex = curAccent
				iconOff, iconOn, iconErr = buildTrayIcons(accentHex)
				lastIconState = ""
			}
		}

		st, err := client.Status()
		store, _ := config.Load()

		activeID := ""
		if store != nil {
			activeID = store.Settings.ActiveProfileID
			setCheckbox(mModeTUN, &modeTUNChecked, store.Settings.Mode == config.ModeTUN)
			setCheckbox(mModeProxy, &modeProxyChecked, store.Settings.Mode != config.ModeTUN)

			setCheckbox(mRoutingOff, &routingOffChecked, !store.Routing.Enabled)
			for i := range routingItems {
				want := store.Routing.Enabled && routingItems[i].id == store.Routing.ActiveProfileID
				setCheckbox(routingItems[i].item, &routingItems[i].checked, want)
			}
		}

		for i := range nodeItems {
			want := false
			if st != nil && st.Running {
				want = nodeItems[i].profile.Name == st.Profile
			} else if activeID != "" {
				want = nodeItems[i].profile.ID == activeID
			}
			setCheckbox(nodeItems[i].item, &nodeItems[i].checked, want)
		}

		var wantIcon, wantTooltip, wantStatus, wantToggle string
		switch {
		case err != nil:
			wantIcon = "err"
			wantTooltip = "Fluffify — нет связи с демоном"
			wantStatus = "✕ Демон не запущен"
			wantToggle = "Подключить"
		case st != nil && st.Running:
			name := st.Profile
			if name == "" {
				name = "active"
			}
			mode := strings.ToUpper(st.Mode)
			wantIcon = "on"
			wantTooltip = fmt.Sprintf("Fluffify — %s (%s)", name, mode)
			wantStatus = fmt.Sprintf("● %s (%s)", name, mode)
			wantToggle = "Отключить"
		default:
			wantIcon = "off"
			wantTooltip = "Fluffify — отключено"
			wantStatus = "○ Отключено"
			wantToggle = "Подключить"
		}

		if wantIcon != lastIconState {
			lastIconState = wantIcon
			switch wantIcon {
			case "on":
				systray.SetIcon(iconOn)
			case "err":
				systray.SetIcon(iconErr)
			default:
				systray.SetIcon(iconOff)
			}
		}
		if wantTooltip != lastTooltip {
			lastTooltip = wantTooltip
			systray.SetTooltip(wantTooltip)
		}
		if wantStatus != lastStatusTitle {
			lastStatusTitle = wantStatus
			mStatus.SetTitle(wantStatus)
		}
		if wantToggle != lastToggleTitle {
			lastToggleTitle = wantToggle
			mToggle.SetTitle(wantToggle)
		}
	}

	connectProfile := func(prof *config.Profile) {
		store, err := config.Load()
		if err != nil {
			return
		}
		if prof == nil {
			prof = store.ActiveProfile()
		}
		if prof == nil && len(store.Profiles) > 0 {
			prof = &store.Profiles[0]
		}
		if prof == nil {
			return
		}
		res, err := generator.BuildForProfile(store, prof)
		if err != nil {
			return
		}
		_, err = client.Start(ipc.StartRequest{
			Profile:   prof.Name,
			Mode:      store.Settings.Mode,
			TUNName:   store.Settings.TUNInterface,
			SocksAddr: res.SocksAddr,
			HTTPAddr:  res.HTTPAddr,
			Mixed:     res.Mixed,
			ProxyAuth: res.ProxyAuth,
			AssetDir:  res.AssetDir,
			Config:    json.RawMessage(res.Config),
		})
		if err == nil {
			store.Settings.ActiveProfileID = prof.ID
			_ = store.Save()
		}
	}

	setMode := func(mode string) {
		store, err := config.Load()
		if err != nil || store.Settings.Mode == mode {
			return
		}
		store.Settings.Mode = mode
		_ = store.Save()
		if st, err := client.Status(); err == nil && st != nil && st.Running {
			connectProfile(nil)
		}
		syncUI()
	}

	setRouting := func(profileID string) {
		store, err := config.Load()
		if err != nil {
			return
		}
		if profileID == "" {
			if !store.Routing.Enabled {
				return
			}
			store.Routing.Enabled = false
		} else {
			if store.Routing.Enabled && store.Routing.ActiveProfileID == profileID {
				return
			}
			store.Routing.Enabled = true
			store.Routing.ActiveProfileID = profileID
		}
		_ = store.Save()
		if st, err := client.Status(); err == nil && st != nil && st.Running {
			connectProfile(nil)
		}
		syncUI()
	}

	syncUI()

	go func() {
		ticker := time.NewTicker(1500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				syncUI()

			case <-mToggle.ClickedCh:
				st, err := client.Status()
				if err == nil && st != nil && st.Running {
					_, _ = client.Stop()
				} else {
					connectProfile(nil)
				}
				syncUI()

			case prof := <-selectProfCh:
				p := prof
				connectProfile(&p)
				syncUI()

			case rpID := <-selectRoutingCh:
				setRouting(rpID)

			case <-mOpen.ClickedCh:
				openTUIWindow()

			case <-mModeTUN.ClickedCh:
				setMode(config.ModeTUN)

			case <-mModeProxy.ClickedCh:
				setMode(config.ModeProxy)

			case <-mQuit.ClickedCh:
				shutdownAll(client)
				return
			}
		}
	}()
}

func openTUIWindow() {
	exe, err := os.Executable()
	if err != nil {
		exe = "xray-tui"
	}
	cmd := exec.Command(exe, "open")
	_ = cmd.Start()
}

// --- Рендер символической SVG-иконки с системным акцентом KDE ---

var (
	reHighlightRule    = regexp.MustCompile(`(?s)\.ColorScheme-Highlight\s*\{[^}]*\}`)
	reCurrentColorCSS  = regexp.MustCompile(`(?i)fill\s*:\s*currentColor`)
	reCurrentColorAttr = regexp.MustCompile(`(?i)fill\s*=\s*"currentColor"`)
	reWhiteFillCSS     = regexp.MustCompile(`(?i)fill\s*:\s*#(?:ffffff|fff)\b`)
)

func buildTrayIcons(accentHex string) (offPNG, onPNG, errPNG []byte) {
	svgRaw := loadSymbolicSVG()

	if b := renderSVGToPNG(colorizeSVG(svgRaw, accentHex, 1.0)); len(b) > 0 {
		onPNG = b
	} else {
		onPNG = makeFallbackIconPNG("on")
	}

	if b := renderSVGToPNG(colorizeSVG(svgRaw, "#5c5c68", 1.0)); len(b) > 0 {
		offPNG = b
	} else {
		offPNG = makeFallbackIconPNG("off")
	}

	if b := renderSVGToPNG(colorizeSVG(svgRaw, "#e04856", 1.0)); len(b) > 0 {
		errPNG = b
	} else {
		errPNG = makeFallbackIconPNG("err")
	}

	return offPNG, onPNG, errPNG
}

func loadSymbolicSVG() []byte {
	home := userHomeDir()
	candidates := []string{
		filepath.Join(home, "xray-tui", "logo", "fluffify-symbolic.svg"),
		filepath.Join(home, ".local", "share", "icons", "hicolor", "symbolic", "apps", "fluffify-symbolic.svg"),
		"/usr/share/icons/hicolor/symbolic/apps/fluffify-symbolic.svg",
	}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "logo", "fluffify-symbolic.svg"))
	}

	for _, p := range candidates {
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			return data
		}
	}
	return []byte(defaultSymbolicSVG)
}

func colorizeSVG(raw []byte, hexColor string, opacity float64) []byte {
	s := string(raw)
	opStr := fmt.Sprintf("%.2f", opacity)
	changed := false

	if reHighlightRule.MatchString(s) {
		s = reHighlightRule.ReplaceAllString(s, fmt.Sprintf(".ColorScheme-Highlight { color: %s; }", hexColor))
		changed = true
	}
	if reCurrentColorCSS.MatchString(s) {
		s = reCurrentColorCSS.ReplaceAllString(s, fmt.Sprintf("fill:%s;fill-opacity:%s", hexColor, opStr))
		changed = true
	}
	if reCurrentColorAttr.MatchString(s) {
		s = reCurrentColorAttr.ReplaceAllString(s, fmt.Sprintf(`fill="%s" fill-opacity="%s"`, hexColor, opStr))
		changed = true
	}
	if !changed && reWhiteFillCSS.MatchString(s) {
		s = reWhiteFillCSS.ReplaceAllString(s, fmt.Sprintf("fill:%s;fill-opacity:%s", hexColor, opStr))
	}
	return []byte(s)
}

func renderSVGToPNG(svgBytes []byte) []byte {
	pngMagic := []byte{0x89, 'P', 'N', 'G'}

	tryCmd := func(name string, args ...string) []byte {
		if _, err := exec.LookPath(name); err != nil {
			return nil
		}
		cmd := exec.Command(name, args...)
		cmd.Stdin = bytes.NewReader(svgBytes)
		out, err := cmd.Output()
		if err == nil && bytes.HasPrefix(out, pngMagic) {
			return out
		}
		return nil
	}

	if out := tryCmd("rsvg-convert", "-w", "64", "-h", "64", "-f", "png"); out != nil {
		return out
	}
	if out := tryCmd("magick", "-background", "none", "svg:-", "-resize", "64x64", "png:-"); out != nil {
		return out
	}
	if out := tryCmd("inkscape", "--pipe", "--export-type=png", "--export-filename=-", "-w", "64", "-h", "64"); out != nil {
		return out
	}
	return nil
}

// readKDEAccentColor извлекает системный акцентный цвет из ~/.config/kdeglobals.
func readKDEAccentColor(path string) string {
	const fallback = "#3daee9"
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}

	var generalAccent, selectionBg, viewActive string
	section := ""

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)

		switch {
		case section == "General" && k == "AccentColor":
			generalAccent = v
		case section == "Colors:Selection" && k == "BackgroundNormal":
			selectionBg = v
		case section == "Colors:View" && k == "ForegroundActive":
			viewActive = v
		}
	}

	for _, raw := range []string{generalAccent, selectionBg, viewActive} {
		if hex := parseKDEColorToHex(raw); hex != "" {
			return hex
		}
	}
	return fallback
}

func parseKDEColorToHex(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "#") {
		h := raw[1:]
		if len(h) == 6 {
			return "#" + h
		}
		if len(h) == 8 {
			return "#" + h[2:]
		}
		return ""
	}

	parts := strings.Split(raw, ",")
	if len(parts) < 3 {
		return ""
	}
	r, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	g, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	b, err3 := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err1 != nil || err2 != nil || err3 != nil {
		return ""
	}
	if r < 0 || r > 255 || g < 0 || g > 255 || b < 0 || b > 255 {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func userHomeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return os.Getenv("HOME")
}

func fileModTime(path string) time.Time {
	if info, err := os.Stat(path); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}

// makeFallbackIconPNG используется только если в системе нет ни rsvg-convert, ни magick, ни inkscape.
func makeFallbackIconPNG(state string) []byte {
	const sz = 32
	const r = 7
	img := image.NewNRGBA(image.Rect(0, 0, sz, sz))

	inRoundedRect := func(x, y int) bool {
		if x < 2 || x >= sz-2 || y < 2 || y >= sz-2 {
			return false
		}
		cx, cy := 0, 0
		if x < 2+r {
			cx = 2 + r
		} else if x >= sz-2-r {
			cx = sz - 3 - r
		}
		if y < 2+r {
			cy = 2 + r
		} else if y >= sz-2-r {
			cy = sz - 3 - r
		}
		if cx != 0 && cy != 0 {
			dx, dy := x-cx, y-cy
			return dx*dx+dy*dy <= r*r
		}
		return true
	}

	for y := 0; y < sz; y++ {
		for x := 0; x < sz; x++ {
			if !inRoundedRect(x, y) {
				continue
			}
			var c color.NRGBA
			switch state {
			case "on":
				c = color.NRGBA{R: 61, G: 174, B: 233, A: 255}
			case "err":
				c = color.NRGBA{R: 220, G: 55, B: 70, A: 255}
			default:
				c = color.NRGBA{R: 92, G: 92, B: 104, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

const defaultSymbolicSVG = `<?xml version="1.0" encoding="UTF-8" standalone="no"?>
<svg width="512" height="512" viewBox="0 0 512 512" version="1.1" xmlns="http://www.w3.org/2000/svg">
  <defs>
    <style id="current-color-scheme" type="text/css">
      .ColorScheme-Highlight { color: #3daee9; }
    </style>
  </defs>
  <g id="layer2">
    <path
       id="text8"
       class="ColorScheme-Highlight"
       style="fill:currentColor"
       d="M 409.48331 88.17918 L 409.48331 99.629553 L 347.95298 99.629553 L 347.95298 129.09985 L 372.70093 129.09985 L 372.70093 118.10549 L 390.32865 118.10549 C 387.61657 127.87761 380.66654 133.22247 347.27388 136.12356 C 351.17251 139.78811 356.08891 146.50584 358.12298 151.54458 L 358.12298 169.56266 L 374.73411 169.56266 L 374.73411 178.8757 L 344.39286 178.8757 L 344.39286 198.26737 L 394.90535 198.26737 C 378.8023 204.9857 359.30965 209.87128 340.664 212.00893 C 345.57967 217.04768 350.66467 224.98922 353.54627 230.63873 C 371.68339 227.12687 391.34533 221.01836 408.29592 213.07852 C 411.68603 218.72802 415.58575 227.43105 416.77229 233.38593 C 431.01079 233.38593 442.19705 233.08079 450.50284 230.027 C 459.14763 227.1259 461.69148 221.62877 461.69148 211.0932 L 461.69148 198.26737 L 500.16749 198.26737 L 500.16749 178.8757 L 461.69148 178.8757 L 461.69148 169.56266 L 489.4892 169.56266 L 489.4892 151.54458 L 368.29297 151.54458 C 403.55019 145.89509 412.87236 135.35938 416.60148 118.10549 L 425.07785 118.10549 L 425.07785 127.26653 C 425.07785 142.99353 429.31582 148.33766 449.31751 148.33766 L 464.06421 148.33766 C 477.62468 148.33766 483.38802 144.2157 485.59159 130.01558 L 480.67534 128.94599 L 496.43863 128.94599 L 496.43863 99.629553 L 434.9083 99.629553 L 434.9083 88.17918 L 409.48331 88.17918 z M 82.183117 90.411048 L 30.857769 400.03084 L 114.8086 400.03084 L 132.26345 294.05233 L 252.78676 294.05233 L 263.79842 227.97309 L 143.27512 227.97309 L 154.91238 158.15306 L 288.7357 158.15306 L 299.95521 90.411048 L 82.183117 90.411048 z M 449.655 118.10549 L 470.33454 118.10549 L 470.33454 125.58707 C 468.46997 124.82362 466.60689 124.06118 465.42035 123.29774 C 464.91183 130.01606 464.06274 131.38918 461.18113 131.38918 L 453.55466 131.38918 C 450.16455 131.38918 449.655 131.08445 449.655 127.11453 L 449.655 118.10549 z M 398.97376 169.56266 L 437.28102 169.56266 L 437.28102 178.8757 L 398.97376 178.8757 L 398.97376 169.56266 z M 433.72297 198.26737 L 437.28102 198.26737 L 437.28102 210.48333 C 437.28102 212.77367 436.26448 213.38438 433.04387 213.38438 C 429.82326 213.38438 417.78878 213.38458 408.63547 212.92652 C 417.95829 208.49853 426.60373 203.6115 433.72297 198.26737 z M 366.74751 255.88627 L 350.30513 269.47583 C 359.45845 274.97264 370.98655 283.21731 376.07172 289.0195 L 393.69944 274.05635 C 387.93625 268.25416 375.90083 260.61964 366.74751 255.88627 z M 479.46943 257.26172 C 459.29823 260.92627 426.07501 262.91095 396.92001 263.52171 C 398.95407 267.33894 401.32774 274.21025 401.83626 278.33286 C 412.00661 278.33286 422.68527 278.0272 433.53364 277.56913 L 433.53364 282.60752 L 390.47887 282.60752 L 390.47887 298.79227 L 420.48057 298.79227 C 410.64922 304.74715 397.42924 309.63273 384.37729 312.53383 C 389.12346 316.19837 395.39508 323.37535 398.61569 328.10871 L 402.34456 326.88711 L 402.34456 338.33934 L 417.09126 338.33934 C 414.71818 348.8749 408.44739 355.44035 388.4457 359.86834 C 392.17483 363.07482 396.75015 368.87721 399.29273 373.45789 C 393.19052 371.32024 388.44444 367.80815 385.05433 362.7694 L 385.05433 324.13806 L 343.5265 324.13806 L 343.5265 344.44732 L 360.98547 344.44732 L 360.98547 364.143 C 353.86622 368.57099 346.06828 372.8471 339.11854 376.35895 L 350.81548 398.95754 C 359.96879 392.3919 367.42553 386.89484 374.54478 381.24533 C 385.39315 393.00241 398.95324 396.97181 419.12443 397.88795 C 440.14317 398.6514 474.21454 398.34506 495.57226 397.27623 C 496.75881 390.86327 500.48871 380.48079 503.20079 375.28936 C 478.96146 377.27432 440.1437 377.73227 419.46398 376.96882 C 413.87029 376.66344 408.95493 376.05349 404.71728 375.13736 C 429.12613 367.65558 436.92313 355.74592 439.63523 338.33934 L 446.92421 338.33934 C 445.73766 343.07271 444.38189 347.50062 443.19534 351.16516 L 464.55192 351.16516 L 465.57057 347.80624 L 473.87614 347.80624 C 473.02861 352.99767 472.01085 355.59259 470.82431 356.66142 C 469.46826 357.88294 467.94235 358.03502 465.39977 358.03502 C 462.51816 358.03502 456.24645 357.88322 449.80522 357.42515 C 452.68683 361.85313 455.06156 368.57115 455.40057 373.6099 C 463.70636 373.91528 471.50346 373.76211 476.08012 373.30404 C 481.3348 372.84597 485.91105 371.62482 489.47068 368.26565 C 493.53882 364.44842 495.57311 356.05021 496.75966 339.40708 C 497.09867 337.11674 497.4367 332.68923 497.4367 332.68923 L 469.46818 332.68923 L 471.8409 322.30659 L 413.53114 322.30659 C 420.65039 318.94742 427.43144 314.82517 433.53364 310.24449 L 433.53364 320.16926 L 456.92546 320.16926 L 456.92546 309.78663 C 466.41779 316.81034 477.77402 322.76421 489.63942 326.12338 C 492.86004 321.23732 499.13166 313.90787 503.87783 310.09064 C 491.33439 307.95299 478.79046 303.83101 469.12863 298.79227 L 499.64067 298.79227 L 499.64067 282.60752 L 456.92546 282.60752 L 456.92546 276.04167 C 470.31642 274.97285 483.0295 273.44612 494.04738 271.46115 L 479.46943 257.26172 z M 359.45853 285.96644 L 343.3557 300.16587 C 352.84802 305.3573 364.8842 313.29817 370.13888 318.94767 L 387.42705 303.37279 C 381.32484 297.72328 368.78136 290.54712 359.45853 285.96644 z "
       transform="scale(0.94910004,1.0536297)" />
  </g>
</svg>`

func shutdownAll(client *ipc.Client) {
	if client != nil {
		_, _ = client.Stop()
	}

	// Штатно останавливаем systemd-юнит, чтобы systemd не перезапустил демон
	_ = exec.Command("systemctl", "--user", "stop", "xray-tui.service").Run()

	// Завершаем оставшиеся процессы этого бинарника (ручной daemon и открытые окна TUI)
	terminateSiblingProcs()

	// Страховка на случай, если systray.Quit() подвиснет на DBus
	time.AfterFunc(2*time.Second, func() {
		os.Exit(0)
	})
	systray.Quit()
}

func terminateSiblingProcs() {
	myPID := os.Getpid()
	exe, _ := os.Executable()
	selfBase := filepath.Base(exe)
	if selfBase == "" || selfBase == "." {
		selfBase = "xray-tui"
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 1 || pid == myPID {
			continue
		}

		matched := false
		if target, err := os.Readlink(filepath.Join("/proc", e.Name(), "exe")); err == nil {
			clean := strings.TrimSuffix(target, " (deleted)")
			if (exe != "" && clean == exe) || filepath.Base(clean) == selfBase {
				matched = true
			}
		}
		if !matched {
			if raw, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline")); err == nil && len(raw) > 0 {
				parts := strings.Split(string(raw), "\x00")
				if len(parts) > 0 && filepath.Base(parts[0]) == selfBase {
					matched = true
				}
			}
		}

		if matched {
			_ = syscall.Kill(pid, syscall.SIGTERM)
		}
	}
}