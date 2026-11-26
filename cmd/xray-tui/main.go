package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"xray-tui/internal/autostart"
	"xray-tui/internal/config"
	"xray-tui/internal/daemon"
	"xray-tui/internal/generator"
	"xray-tui/internal/geo"
	"xray-tui/internal/ipc"
	"xray-tui/internal/osutil"
	"xray-tui/internal/parser"
	"xray-tui/internal/ping"
	"xray-tui/internal/tray"
	"xray-tui/internal/tui"
)

func main() {
	// В Windows включает ANSI-режим и UTF-8: без этого кадры TUI «плывут».
	osutil.EnableVirtualTerminal()

	cmd := "open"
	var args []string
	if len(os.Args) >= 2 {
		cmd = os.Args[1]
		args = os.Args[2:]
	}

	if err := run(cmd, args); err != nil {
		fmt.Fprintf(os.Stderr, "ошибка: %v\n", err)
		os.Exit(1)
	}
}

func run(cmd string, args []string) error {
	sock := ipc.DefaultSocketPath()

	switch cmd {
	case "daemon":
		// Демон обязан работать с правами администратора: если какой-то путь
		// запуска поднял его без прав, он сам перезапускается elevated.
		if osutil.SelfElevateDaemon() {
			return nil
		}
		ctx, stop := signal.NotifyContext(context.Background(), osutil.ShutdownSignals()...)
		defer stop()
		return daemon.New(sock).Run(ctx)

	case "daemon-hidden":
		// Тот же демон, но сразу прячет своё окно консоли: так его запускает
		// UAC-перезапуск (команды restart-admin и самоподнятие демона).
		if osutil.SelfElevateDaemon() {
			return nil
		}
		osutil.HideConsoleIfOwned()
		ctx, stop := signal.NotifyContext(context.Background(), osutil.ShutdownSignals()...)
		defer stop()
		return daemon.New(sock).Run(ctx)

	case "up", "start":
		return runUp(sock, args)
	case "tray":
		tray.Run(sock)
		return nil
	case "tui":
		ensureDaemonRunning(sock)
		if tui.HasGUI() {
			tray.EnsureRunning()
		}
		return tui.Run(sock)

	case "down", "stop":
		if _, err := ipc.NewClient(sock).Stop(); err != nil {
			return err
		}
		fmt.Println("Xray остановлен")
		return nil

	case "status":
		st, err := ipc.NewClient(sock).Status()
		if err != nil {
			return err
		}
		if !st.Running {
			fmt.Println("Статус: остановлен")
			return nil
		}
		uptime := (time.Duration(st.UptimeSec) * time.Second).String()
		mode := st.Mode
		if mode == "" {
			mode = "proxy"
		}
		fmt.Printf("Статус: работает\nРежим: %s\nПрофиль: %s\n", mode, st.Profile)
		if st.Mixed {
			fmt.Printf("Mixed (SOCKS+HTTP): %s\n", st.SocksAddr)
		} else {
			fmt.Printf("SOCKS5: %s\n", st.SocksAddr)
			if st.HTTPAddr != "" {
				fmt.Printf("HTTP:   %s\n", st.HTTPAddr)
			}
		}
		if st.ProxyAuth {
			fmt.Println("Авторизация: вкл")
		}
		fmt.Printf("Аптайм: %s\n", uptime)
		return nil

	case "logs":
		client := ipc.NewClient(sock)
		if len(args) > 0 && (args[0] == "-f" || args[0] == "--follow") {
			ctx, stop := signal.NotifyContext(context.Background(), osutil.ShutdownSignals()...)
			defer stop()
			return client.StreamLogs(ctx, func(line string) {
				fmt.Println(line)
			})
		}
		lines, err := client.Logs()
		if err != nil {
			return err
		}
		for _, line := range lines {
			fmt.Println(line)
		}
		return nil

	case "add":
		return runAdd(args)

	case "list", "ls":
		return runList()

	case "rm":
		return runRemove(args)

	case "sub":
		return runSub(args)

	case "hwid":
		return runHWID(args)

	case "parse":
		return runParse(args)

	case "-h", "--help", "help":
		printUsage()
		return nil

	case "ping":
		return runPing(args)

	case "restart-admin":
		return runRestartAdmin(sock)

	case "open", "win":
		return runOpen(sock)

	case "tun-setup":
		return runTUNSetup()

	case "geo":
		return runGeo(args)

	default:
		printUsage()
		return fmt.Errorf("неизвестная команда: %s", cmd)
	}
}

func runUp(sock string, args []string) error {
	fs := flag.NewFlagSet("up", flag.ContinueOnError)
	cfgPath := fs.String("c", "", "путь к Raw JSON конфигу")
	uri := fs.String("u", "", "ссылка (vless://, hy2://, trojan:// и т. д.)")

	target, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if target == "" && fs.NArg() > 0 {
		target = fs.Arg(0)
	}

	if *cfgPath != "" && *uri != "" {
		return errors.New("укажи либо -c, либо -u, но не оба сразу")
	}

	store, err := config.Load()
	if err != nil {
		return err
	}
	opts := generator.OptionsFromStore(store)
	var req ipc.StartRequest

	switch {
	case *cfgPath != "":
		raw, err := os.ReadFile(*cfgPath)
		if err != nil {
			return fmt.Errorf("не удалось прочитать %s: %w", *cfgPath, err)
		}
		patched, socksAddr, httpAddr, err := generator.PrepareRawJSON(raw, opts)
		if err != nil {
			return err
		}
		req = ipc.StartRequest{
			Profile:   filepath.Base(*cfgPath),
			SocksAddr: socksAddr,
			HTTPAddr:  httpAddr,
			Mixed:     opts.MixedInbound,
			ProxyAuth: opts.ProxyAuth && opts.ProxyUser != "",
			Config:    json.RawMessage(patched),
		}

	case *uri != "":
		node, err := parser.ParseURI(*uri)
		if err != nil {
			return fmt.Errorf("ошибка парсинга ссылки: %w", err)
		}
		cfg, err := generator.BuildFromNode(node, opts)
		if err != nil {
			return fmt.Errorf("ошибка генерации конфига: %w", err)
		}
		socksAddr := net.JoinHostPort(opts.Listen, strconv.Itoa(opts.SocksPort))
		httpAddr := ""
		if !opts.MixedInbound && opts.HTTPPort > 0 {
			httpAddr = net.JoinHostPort(opts.Listen, strconv.Itoa(opts.HTTPPort))
		}
		req = ipc.StartRequest{
			Profile:   node.Name,
			SocksAddr: socksAddr,
			HTTPAddr:  httpAddr,
			Mixed:     opts.MixedInbound,
			ProxyAuth: opts.ProxyAuth && opts.ProxyUser != "",
			Config:    json.RawMessage(cfg),
		}

	default:
		var prof *config.Profile
		if target != "" {
			prof, _, err = store.FindProfile(target)
			if err != nil {
				return err
			}
		} else {
			prof = store.ActiveProfile()
		}

		if prof != nil {
			res, err := generator.BuildForProfile(store, prof)
			if err != nil {
				return err
			}
			store.Settings.ActiveProfileID = prof.ID
			_ = store.Save()
			req = ipc.StartRequest{
				Profile:   prof.Name,
				SocksAddr: res.SocksAddr,
				HTTPAddr:  res.HTTPAddr,
				Mixed:     res.Mixed,
				ProxyAuth: res.ProxyAuth,
				AssetDir:  res.AssetDir,
				Config:    json.RawMessage(res.Config),
			}
		}
	}

	req.Mode = store.Settings.Mode
	req.TUNName = store.Settings.TUNInterface

	st, err := ipc.NewClient(sock).Start(req)
	if err != nil {
		return err
	}
	fmt.Printf("Xray запущен\nПрофиль: %s\nSOCKS5: %s\n", st.Profile, st.SocksAddr)
	if st.HTTPAddr != "" && !st.Mixed {
		fmt.Printf("HTTP:   %s\n", st.HTTPAddr)
	}
	return nil
}

func measureProfilePing(store *config.Store, prof *config.Profile) (time.Duration, error) {
	if store.Settings.PingMode == config.PingModeTCP {
		addr, port := prof.Address, prof.Port
		if prof.Type == config.ProfileTypeRaw {
			if a, p, err := ping.ExtractEndpoint(prof.RawJSON); err == nil {
				addr, port = a, p
			}
		}
		return ping.MeasureTCP(context.Background(), addr, port)
	}
	if store.Settings.PingMode == config.PingModeICMP {
		addr := prof.Address
		if prof.Type == config.ProfileTypeRaw {
			if a, _, err := ping.ExtractEndpoint(prof.RawJSON); err == nil && a != "" {
				addr = a
			}
		}
		return ping.MeasureICMP(context.Background(), addr, "")
	}

	var cfg []byte
	var err error
	if prof.Type == config.ProfileTypeRaw {
		cfg = prof.RawJSON
	} else {
		node, parseErr := parser.ParseURI(prof.URI)
		if parseErr != nil {
			return 0, parseErr
		}
		opts := generator.DefaultOptions()
		opts.LogLevel = "error"
		cfg, err = generator.BuildFromNode(node, opts)
		if err != nil {
			return 0, err
		}
	}
	return ping.Measure(context.Background(), cfg, store.Settings.PingURL)
}

func runAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	customName := fs.String("name", "", "своё имя профиля")

	input, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if input == "" && fs.NArg() > 0 {
		input = fs.Arg(0)
	}
	if input == "" {
		return fmt.Errorf("укажи ссылку, JSON или путь к файлу: %s add '<uri|node.json>' [-name 'Имя']", osutil.SelfName())
	}

	store, err := config.Load()
	if err != nil {
		return err
	}

	kind, value := parser.ClassifyInput(input)
	if kind == parser.InputSubscription {
		fmt.Println("Это ссылка подписки — добавляю как подписку.")
		subArgs := []string{value}
		if *customName != "" {
			subArgs = append(subArgs, "-name", *customName)
		}
		return runSubAdd(subArgs)
	}

	var profs []config.Profile
	addURI := func(uri string) error {
		node, err := parser.ParseURI(uri)
		if err != nil {
			return err
		}
		name := node.Name
		if *customName != "" {
			name = *customName
		}
		profs = append(profs, config.Profile{
			ID:       config.NewID(),
			Name:     name,
			Type:     config.ProfileTypeURI,
			Protocol: node.Protocol,
			Address:  node.Address,
			Port:     node.Port,
			URI:      uri,
			AddedAt:  time.Now(),
		})
		return nil
	}
	addRaw := func(raw []byte, fileName string) error {
		item, ok := parser.InspectRawConfig(json.RawMessage(raw), fileName)
		if !ok {
			return errors.New("не является валидным Xray JSON конфигом (отсутствует outbounds)")
		}
		name := item.Name
		if *customName != "" {
			name = *customName
		}
		profs = append(profs, config.Profile{
			ID:       config.NewID(),
			Name:     name,
			Type:     config.ProfileTypeRaw,
			Protocol: item.Protocol,
			Address:  item.Address,
			Port:     item.Port,
			RawJSON:  item.RawJSON,
			AddedAt:  time.Now(),
		})
		return nil
	}

	switch kind {
	case parser.InputLink:
		for _, line := range parser.TotalLines(value) {
			if err := addURI(line); err != nil {
				return err
			}
		}
	case parser.InputJSON:
		if err := addRaw([]byte(value), "вставленный JSON"); err != nil {
			return err
		}
	case parser.InputFilePath:
		raw, err := os.ReadFile(value)
		if err != nil {
			return fmt.Errorf("не удалось прочитать файл %s: %w", value, err)
		}
		contentKind, content := parser.ClassifyInput(string(raw))
		switch contentKind {
		case parser.InputJSON:
			if err := addRaw([]byte(content), filepath.Base(value)); err != nil {
				return err
			}
		case parser.InputLink:
			for _, line := range parser.TotalLines(content) {
				if err := addURI(line); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("%s не похож ни на JSON, ни на список ссылок", filepath.Base(value))
		}
	default:
		return fmt.Errorf("не понял ввод %q: нужна ссылка, JSON или путь к файлу", input)
	}

	if len(profs) == 0 {
		return errors.New("ничего не добавлено")
	}
	store.Profiles = append(store.Profiles, profs...)
	if err := store.Save(); err != nil {
		return err
	}
	for _, p := range profs {
		fmt.Printf("Добавлен профиль: %s [%s]\n", p.Name, p.Protocol)
	}
	return nil
}

func runList() error {
	store, err := config.Load()
	if err != nil {
		return err
	}
	if len(store.Profiles) == 0 {
		fmt.Printf("Профилей пока нет. Добавь через '%s add' или '%s sub add'.\n", osutil.SelfName(), osutil.SelfName())
		return nil
	}

	for i, p := range store.Profiles {
		mark := " "
		if p.ID == store.Settings.ActiveProfileID {
			mark = "*"
		}
		source := store.SubName(p.SubID)
		fmt.Printf("%s %2d) [%-11s] %s (%s)\n", mark, i+1, p.Protocol, p.Name, source)
	}
	return nil
}

func runRemove(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("укажи номер или имя профиля: %s rm <номер|имя>", osutil.SelfName())
	}
	store, err := config.Load()
	if err != nil {
		return err
	}
	p, idx, err := store.FindProfile(args[0])
	if err != nil {
		return err
	}
	name := p.Name
	store.RemoveProfile(idx)
	if err := store.Save(); err != nil {
		return err
	}
	fmt.Printf("Удалён профиль: %s\n", name)
	return nil
}

func runSub(args []string) error {
	if len(args) < 1 {
		printSubUsage()
		return nil
	}

	switch args[0] {
	case "add":
		return runSubAdd(args[1:])
	case "update", "up":
		return runSubUpdate(args[1:])
	case "list", "ls":
		return runSubList()
	case "rm", "del":
		return runSubRemove(args[1:])
	default:
		printSubUsage()
		return fmt.Errorf("неизвестная подкоманда sub: %s", args[0])
	}
}

func runSubAdd(args []string) error {
	fs := flag.NewFlagSet("sub add", flag.ContinueOnError)
	name := fs.String("name", "", "имя подписки (если пусто — берётся из profile-title)")
	ua := fs.String("ua", "", "кастомный User-Agent (например, Happ/2.0 или v2rayN/7.0)")
	noHWID := fs.Bool("no-hwid", false, "не отправлять HWID-заголовки")
	customHWID := fs.String("hwid", "", "свой HWID для этой подписки")

	subURL, flagArgs := extractPositional(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if subURL == "" && fs.NArg() > 0 {
		subURL = fs.Arg(0)
	}
	if subURL == "" {
		return fmt.Errorf("укажи URL подписки: %s sub add '<url>' [-name ...] [-ua ...]", osutil.SelfName())
	}

	store, err := config.Load()
	if err != nil {
		return err
	}

	sub := config.Subscription{
		ID:         config.NewID(),
		Name:       *name,
		URL:        subURL,
		UserAgent:  *ua,
		SendHWID:   !*noHWID,
		CustomHWID: *customHWID,
	}

	if err := syncSubscription(store, &sub); err != nil {
		return err
	}
	if sub.Name == "" {
		sub.Name = fmt.Sprintf("sub-%d", len(store.Subscriptions)+1)
	}

	store.Subscriptions = append(store.Subscriptions, sub)
	if err := store.Save(); err != nil {
		return err
	}
	return nil
}

func runSubUpdate(args []string) error {
	store, err := config.Load()
	if err != nil {
		return err
	}
	if len(store.Subscriptions) == 0 {
		return errors.New("подписок пока нет")
	}

	if len(args) > 0 && args[0] != "all" {
		sub, idx, err := store.FindSub(args[0])
		if err != nil {
			return err
		}
		if err := syncSubscription(store, sub); err != nil {
			return err
		}
		store.Subscriptions[idx] = *sub
		return store.Save()
	}

	var failed int
	for i := range store.Subscriptions {
		if err := syncSubscription(store, &store.Subscriptions[i]); err != nil {
			fmt.Fprintf(os.Stderr, "[%s] ошибка: %v\n", store.Subscriptions[i].Name, err)
			failed++
		}
	}
	if err := store.Save(); err != nil {
		return err
	}
	if failed > 0 {
		return fmt.Errorf("обновлено с ошибками (%d)", failed)
	}
	return nil
}

func syncSubscription(store *config.Store, sub *config.Subscription) error {
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

	label := sub.Name
	if label == "" {
		label = sub.URL
	}
	fmt.Printf("Подписка %q обновлена: загружено %d нод", label, len(profiles))
	if res.Skipped > 0 {
		fmt.Printf(" (пропущено: %d)", res.Skipped)
	}
	fmt.Println()
	for _, w := range res.Warnings {
		fmt.Printf("  предупреждение: %s\n", w)
	}
	if sub.Info != nil {
		fmt.Printf("  трафик: %s\n", formatSubInfo(sub.Info))
	}
	return nil
}

func runSubList() error {
	store, err := config.Load()
	if err != nil {
		return err
	}
	if len(store.Subscriptions) == 0 {
		fmt.Println("Подписок пока нет.")
		return nil
	}

	for i, sub := range store.Subscriptions {
		count := 0
		for _, p := range store.Profiles {
			if p.SubID == sub.ID {
				count++
			}
		}
		hwidState := "off"
		if sub.SendHWID {
			hwidState = "on"
		}
		ua := sub.UserAgent
		if ua == "" {
			ua = store.Settings.DefaultUA
		}
		fmt.Printf("%2d) %s — %d нод | HWID: %s | UA: %s\n", i+1, sub.Name, count, hwidState, ua)
		if sub.Info != nil {
			fmt.Printf("    %s\n", formatSubInfo(sub.Info))
		}
	}
	return nil
}

func runSubRemove(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("укажи номер или имя подписки: %s sub rm <номер|имя>", osutil.SelfName())
	}
	store, err := config.Load()
	if err != nil {
		return err
	}
	sub, idx, err := store.FindSub(args[0])
	if err != nil {
		return err
	}
	name := sub.Name
	removedNodes := store.RemoveSub(idx)
	if err := store.Save(); err != nil {
		return err
	}
	fmt.Printf("Удалена подписка %q и %d её нод\n", name, removedNodes)
	return nil
}

func runHWID(args []string) error {
	store, err := config.Load()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		fmt.Printf("HWID: %s\nOS: %s %s | Model: %s\n",
			store.Settings.HWID, osutil.OSName(), config.DeviceOSVersion(), config.DeviceModel())
		return nil
	}
	store.Settings.HWID = strings.TrimSpace(args[0])
	if err := store.Save(); err != nil {
		return err
	}
	fmt.Printf("Новый HWID сохранён: %s\n", store.Settings.HWID)
	return nil
}

func runParse(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("укажи ссылку: %s parse '<uri>'", osutil.SelfName())
	}
	node, err := parser.ParseURI(args[0])
	if err != nil {
		return err
	}
	data, err := generator.BuildFromNode(node, generator.DefaultOptions())
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// extractPositional позволяет передавать флаги как до, так и после основного аргумента.
func extractPositional(args []string) (string, []string) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return args[0], args[1:]
	}
	return "", args
}

func formatSubInfo(info *config.SubInfo) string {
	used := info.Upload + info.Download
	parts := []string{fmt.Sprintf("использовано %s", formatBytes(used))}
	if info.Total > 0 {
		parts[0] = fmt.Sprintf("%s / %s", formatBytes(used), formatBytes(info.Total))
	}
	if info.Expire > 0 {
		parts = append(parts, "до "+time.Unix(info.Expire, 0).Format("2006-01-02"))
	}
	return strings.Join(parts, " | ")
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
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func printUsage() {
	fmt.Printf("Использование: %s <команда>\n", osutil.SelfName())
	fmt.Print(`
Прокси:
  daemon                         Запустить фоновый демон (локальный сокет)
  up [номер|имя] [-u uri] [-c f] Запустить или переключить профиль
  down                           Остановить прокси
  status                         Показать статус и активный профиль
  logs [-f]                      Показать логи (-f — в реальном времени)

Профили и подписки:
  list                           Список всех сохранённых профилей
  add <uri|node.json> [-name n]  Добавить ссылку, подписку или Raw JSON
  rm <номер|имя>                 Удалить профиль
  sub <add|update|list|rm>       Управление подписками
  hwid [значение]                Показать или задать HWID для подписок
  parse '<uri>'                  Разобрать ссылку и вывести JSON для xray

TUN:
  tun-setup                      Проверить права для TUN-режима
  restart-admin                  Перезапустить демон от администратора (UAC)
`)
}

func printSubUsage() {
	fmt.Printf("Использование: %s sub <подкоманда>\n", osutil.SelfName())
	fmt.Print(`
  sub add <url> [-name n] [-ua ua] [-hwid id] [-no-hwid]
  sub update [номер|имя]
  sub list
  sub rm <номер|имя>
`)
}

func runPing(args []string) error {
	store, err := config.Load()
	if err != nil {
		return err
	}

	if len(args) > 0 && args[0] == "all" {
		return runPingAll(store)
	}

	var prof *config.Profile
	if len(args) > 0 {
		prof, _, err = store.FindProfile(args[0])
		if err != nil {
			return err
		}
	} else {
		prof = store.ActiveProfile()
	}
	if prof == nil {
		return fmt.Errorf("укажи профиль или 'all': %s ping <номер|имя|all>", osutil.SelfName())
	}

	rtt, err := measureProfilePing(store, prof)
	if err != nil {
		fmt.Printf("%s — ошибка (%v)\n", prof.Name, err)
		return nil
	}
	fmt.Printf("%s — %d ms (%s)\n", prof.Name, rtt.Milliseconds(), store.Settings.PingMode)
	return nil
}

func runPingAll(store *config.Store) error {
	if len(store.Profiles) == 0 {
		fmt.Println("Серверов пока нет.")
		return nil
	}

	type res struct {
		idx int
		p   config.Profile
		ms  int64
		err error
	}

	workers := 4
	if store.Settings.PingMode == config.PingModeTCP || store.Settings.PingMode == config.PingModeICMP {
		workers = 16
	}
	sem := make(chan struct{}, workers)
	results := make([]res, len(store.Profiles))

	var wg sync.WaitGroup
	for i, p := range store.Profiles {
		wg.Add(1)
		go func(idx int, prof config.Profile) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			rtt, err := measureProfilePing(store, &prof)
			results[idx] = res{idx: idx + 1, p: prof, ms: rtt.Milliseconds(), err: err}
		}(i, p)
	}
	wg.Wait()

	if store.Settings.AutoSortPing {
		sort.SliceStable(results, func(i, j int) bool {
			if (results[i].err == nil) != (results[j].err == nil) {
				return results[i].err == nil
			}
			if results[i].err == nil {
				return results[i].ms < results[j].ms
			}
			return false
		})
	}

	for _, r := range results {
		if r.err != nil {
			fmt.Printf("%2d) [err    ] %s\n", r.idx, r.p.Name)
		} else {
			fmt.Printf("%2d) [%4d ms] %s\n", r.idx, r.ms, r.p.Name)
		}
	}
	return nil
}

func runGeo(args []string) error {
	store, err := config.Load()
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "update" {
		for _, p := range store.Routing.GeoProviders {
			fmt.Printf("Скачиваю %s... ", p.Name)
			st, err := geo.Download(context.Background(), p, config.GeoProviderDir(p.ID))
			if err != nil {
				fmt.Printf("ошибка: %v\n", err)
				continue
			}
			fmt.Printf("готово (%s)\n", formatBytes(st.TotalBytes))
		}
		return nil
	}

	for _, p := range store.Routing.GeoProviders {
		st := geo.Check(config.GeoProviderDir(p.ID))
		if st.Ready() {
			fmt.Printf("%-16s скачаны (%s, %s)\n", p.Name, formatBytes(st.TotalBytes), st.UpdatedAt.Format("2006-01-02 15:04"))
		} else {
			fmt.Printf("%-16s не скачаны\n", p.Name)
		}
	}
	return nil
}

func runOpen(sock string) error {
	osutil.HideConsoleIfOwned()
	ensureDaemonRunning(sock)
	if !tui.HasGUI() {
		return tui.Run(sock)
	}
	tray.EnsureRunning()
	store, err := config.Load()
	if err != nil {
		return err
	}
	if err := tui.LaunchWindow(store.Settings.WindowCols, store.Settings.WindowLines, store.Settings.Terminal); err != nil {
		return tui.Run(sock)
	}
	return nil
}

func ensureDaemonRunning(sock string) {
	client := ipc.NewClient(sock)
	if _, err := client.Status(); err == nil {
		return
	}

	// 1. Пробуем поднять через автозапуск (systemd --user, если юнит установлен)
	if autostart.Start() {
		if waitForDaemon(client, 1500*time.Millisecond) {
			return
		}
	}

	// 2. Windows: если демона нет — поднимаем его только с правами администратора.
	//    Обычный (без прав) демон не оставляем: он держал бы сокет и ТУ�? требовал
	//    бы ручного ремонта. Если UAC отклонён — демона нет, и следующий запуск
	//    снова запросит права.
	if osutil.TunRequiresAdmin() {
		fmt.Fprintln(os.Stderr, "Запускаю демон с правами администратора (запрос UAC)...")
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		startErr := daemon.StartElevated(ctx, sock)
		cancel()
		if startErr != nil {
			fmt.Fprintf(os.Stderr, "демон не запущен (%v) — запусти приложение заново и подтверди UAC\n", startErr)
		}
		return
	}

	// 3. Фоллбэк (Linux без systemd): запускаем демон напрямую в отвязанной сессии
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "daemon")
	osutil.Detach(cmd)
	if devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0); err == nil {
		cmd.Stdin = devNull
		cmd.Stdout = devNull
		cmd.Stderr = devNull
		defer devNull.Close()
	}
	if err := cmd.Start(); err == nil {
		waitForDaemon(client, 1500*time.Millisecond)
	}
}

// runRestartAdmin останавливает демон и поднимает его заново от администратора:
// в Windows это нужно для TUN-режима (UAC-запрос).
func runRestartAdmin(sock string) error {
	ctx, stop := signal.NotifyContext(context.Background(), osutil.ShutdownSignals()...)
	defer stop()

	if err := daemon.RestartElevated(ctx, sock); err != nil {
		return err
	}
	fmt.Println("Демон перезапущен с правами администратора — TUN доступен.")
	return nil
}

func waitForDaemon(client *ipc.Client, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := client.Status(); err == nil {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
