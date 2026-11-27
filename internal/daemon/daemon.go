package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"xray-tui/internal/applog"
	"xray-tui/internal/config"
	"xray-tui/internal/core"
	"xray-tui/internal/generator"
	"xray-tui/internal/ipc"
	"xray-tui/internal/netcheck"
	"xray-tui/internal/osutil"
)

const tempTestConfig = `{
  "log": {
    "loglevel": "warning"
  },
  "inbounds": [
    {
      "tag": "socks-in",
      "listen": "127.0.0.1",
      "port": 10808,
      "protocol": "socks",
      "settings": {
        "udp": true
      }
    }
  ],
  "outbounds": [
    {
      "tag": "direct",
      "protocol": "freedom"
    }
  ]
}`

// tunRequiresAdmin вынесено в переменную, чтобы тесты могли проверить отказ
// на TUN без прав администратора.
var tunRequiresAdmin = osutil.TunRequiresAdmin

type Daemon struct {
	socketPath string
	runner     *core.Runner
	xrayBin    string

	mu            sync.Mutex
	startedAt     time.Time
	activeProfile string
	socksAddr     string
	httpAddr      string
	mixed         bool
	proxyAuth     bool
	mode          string
	tunName       string
	elevated      bool

	// Dev-режим: логи ядра на info + зеркало в файл. lastConfig —
	// последний конфиг, присланный клиентом (без dev-патча уровня).
	devLog       bool
	logMirror    *os.File
	lastConfig   []byte
	lastAssetDir string
	version      string

	subsMu sync.RWMutex
	subs   map[chan string]struct{}
	stopCh chan struct{}
	quitCh chan struct{}
	// quitOnce защищает от повторного закрытия quitCh.
	quitOnce sync.Once
}

func New(socketPath, xrayBin string) *Daemon {
	if socketPath == "" {
		socketPath = ipc.DefaultSocketPath()
	}

	d := &Daemon{
		socketPath: socketPath,
		xrayBin:    xrayBin,
		subs:       make(map[chan string]struct{}),
		stopCh:     make(chan struct{}),
		quitCh:     make(chan struct{}),
		elevated:   osutil.IsElevated(),
	}
	d.runner = core.NewRunner(xrayBin, d.broadcastLog)
	return d
}

func (d *Daemon) Run(ctx context.Context) error {
	applog.Init("daemon")
	if err := os.MkdirAll(filepath.Dir(d.socketPath), 0o700); err != nil {
		return fmt.Errorf("не удалось создать директорию сокета: %w", err)
	}

	probeCtx, cancelProbe := context.WithTimeout(ctx, 300*time.Millisecond)
	conn, err := ipc.Dial(probeCtx, d.socketPath)
	cancelProbe()
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("демон уже запущен на %s", d.socketPath)
	}
	_ = os.Remove(d.socketPath)

	ln, err := ipc.Listen(d.socketPath)
	if err != nil {
		return fmt.Errorf("не удалось открыть сокет %s: %w", d.socketPath, err)
	}
	_ = ipc.SecureSocket(d.socketPath)
	defer func() {
		_ = ln.Close()
		_ = os.Remove(d.socketPath)
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", d.handleStatus)
	mux.HandleFunc("POST /start", d.handleStart)
	mux.HandleFunc("POST /stop", d.handleStop)
	mux.HandleFunc("POST /quit", d.handleQuit)
	mux.HandleFunc("GET /logs", d.handleLogs)
	mux.HandleFunc("GET /logs/stream", d.handleLogsStream)
	mux.HandleFunc("POST /devlog", d.handleDevLog)
	mux.HandleFunc("GET /devstatus", d.handleDevStatus)
	mux.HandleFunc("GET /devconfig", d.handleDevConfig)
	mux.HandleFunc("POST /devcheck", d.handleDevCheck)

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	fmt.Printf("[daemon] слушает %s\n", d.socketPath)
	applog.Event("daemon listening", "socket", d.socketPath, "elevated", d.elevated)
	go d.tryAutoConnect()

	select {
	case <-ctx.Done():
		fmt.Println("\n[daemon] завершение работы...")
	case <-d.quitCh:
		fmt.Println("\n[daemon] завершение по запросу клиента...")
	case err := <-errCh:
		return err
	}

	close(d.stopCh)
	_ = d.runner.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func (d *Daemon) tryAutoConnect() {
	store, err := config.Load()
	if err != nil || !store.Settings.AutoConnect {
		return
	}
	prof := store.ActiveProfile()
	if prof == nil {
		return
	}

	res, err := generator.BuildForProfile(store, prof)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[daemon] автоподключение пропущено: %v\n", err)
		return
	}
	if cfg := d.devPatchedConfig(res.Config); cfg != nil {
		res.Config = cfg
	}
	d.mu.Lock()
	d.lastConfig = append([]byte(nil), res.Config...)
	d.lastAssetDir = res.AssetDir
	d.mu.Unlock()

	if err := d.runner.Start(res.Config, res.AssetDir); err != nil {
		fmt.Fprintf(os.Stderr, "[daemon] ошибка автоподключения: %v\n", err)
		applog.Fail("autoconnect failed", err, "profile", prof.Name)
		return
	}

	d.mu.Lock()
	d.startedAt = time.Now()
	d.activeProfile = prof.Name
	d.socksAddr = res.SocksAddr
	d.httpAddr = res.HTTPAddr
	d.mixed = res.Mixed
	d.proxyAuth = res.ProxyAuth
	d.mode = store.Settings.Mode
	d.tunName = store.Settings.TUNInterface
	d.mu.Unlock()

	fmt.Printf("[daemon] автоподключение: %s (%s)\n", prof.Name, store.Settings.Mode)
	applog.Event("autoconnect", "profile", prof.Name, "mode", store.Settings.Mode)
}

func (d *Daemon) broadcastLog(line string) {
	d.mu.Lock()
	if d.devLog && d.logMirror != nil {
		fmt.Fprintln(d.logMirror, line)
	}
	d.mu.Unlock()

	d.subsMu.RLock()
	defer d.subsMu.RUnlock()

	for ch := range d.subs {
		select {
		case ch <- line:
		default:
		}
	}
}

func (d *Daemon) statusSnapshot() ipc.StatusResponse {
	running := d.runner.IsRunning()

	d.mu.Lock()
	startedAt := d.startedAt
	profile := d.activeProfile
	socksAddr := d.socksAddr
	httpAddr := d.httpAddr
	mixed := d.mixed
	proxyAuth := d.proxyAuth
	mode := d.mode
	tunName := d.tunName
	d.mu.Unlock()

	resp := ipc.StatusResponse{Running: running, Elevated: d.elevated}
	if running {
		resp.Profile = profile
		resp.Mode = mode
		resp.TUNName = tunName
		resp.SocksAddr = socksAddr
		resp.HTTPAddr = httpAddr
		resp.Mixed = mixed
		resp.ProxyAuth = proxyAuth
		resp.StartedAt = startedAt
		resp.UptimeSec = int64(time.Since(startedAt).Seconds())
	}
	return resp
}

func (d *Daemon) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, d.statusSnapshot())
}

func (d *Daemon) handleStart(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ipc.ErrorResponse{Error: "не удалось прочитать запрос"})
		return
	}

	var req ipc.StartRequest
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, ipc.ErrorResponse{Error: "невалидный JSON запроса"})
			return
		}
	}

	cfg := bytes.TrimSpace(req.Config)
	profile := req.Profile
	socksAddr := req.SocksAddr
	mode := req.Mode
	if mode == "" {
		mode = "proxy"
	}
	tunName := req.TUNName
	if tunName == "" {
		tunName = "xray0"
	}

	// В Windows TUN создаёт виртуальный адаптер, а это доступно только
	// процессу с правами администратора. В Linux права выданы бинарнику xray.
	if mode == "tun" && tunRequiresAdmin() && !d.elevated {
		writeJSON(w, http.StatusConflict, ipc.ErrorResponse{Error: fmt.Sprintf(
			"TUN требует прав администратора — перезапусти демон командой: %s restart-admin",
			osutil.SelfName())})
		return
	}

	if len(cfg) == 0 {
		cfg = []byte(tempTestConfig)
		if profile == "" {
			profile = "test-direct"
		}
		if socksAddr == "" {
			socksAddr = "127.0.0.1:10808"
		}
	} else if !json.Valid(cfg) {
		writeJSON(w, http.StatusBadRequest, ipc.ErrorResponse{Error: "передан невалидный JSON конфиг"})
		return
	}
	if socksAddr == "" {
		socksAddr = "127.0.0.1:10808"
	}

	// Последний конфиг запоминаем в исходном виде: dev-патч уровня логов
	// накладывается при каждом старте ядра, а дамп (/devconfig) показывает
	// фактический (запатченный) вариант.
	d.mu.Lock()
	devLog := d.devLog
	if len(bytes.TrimSpace(req.Config)) > 0 {
		d.lastConfig = append([]byte(nil), cfg...)
		d.lastAssetDir = req.AssetDir
	}
	d.mu.Unlock()
	if devLog {
		cfg = patchLogLevel(cfg, "info")
	}

	if d.runner.IsRunning() {
		if err := d.runner.Stop(); err != nil {
			writeJSON(w, http.StatusInternalServerError, ipc.ErrorResponse{Error: err.Error()})
			return
		}
	}

	if err := d.runner.Start(cfg, req.AssetDir); err != nil {
		writeJSON(w, http.StatusConflict, ipc.ErrorResponse{Error: err.Error()})
		applog.Fail("core start failed", err, "profile", profile)
		return
	}

	time.Sleep(250 * time.Millisecond)
	if !d.runner.IsRunning() {
		reason := ""
		logs := d.runner.Logs()
		for i := len(logs) - 1; i >= 0; i-- {
			l := strings.TrimSpace(logs[i])
			if l == "" || strings.HasPrefix(l, "[runner]") {
				continue
			}
			reason = l
			break
		}

		errMsg := "xray завершился после старта"
		if mode == "tun" && (strings.Contains(strings.ToLower(reason), "permission") ||
			strings.Contains(strings.ToLower(reason), "not permitted") ||
			strings.Contains(strings.ToLower(reason), "no such device") ||
			strings.Contains(strings.ToLower(reason), "tun")) {
			errMsg = "TUN не запустился — выполни: " + osutil.SelfName() + " tun-setup"
		}
		if reason != "" {
			errMsg += ": " + reason
		}
		writeJSON(w, http.StatusInternalServerError, ipc.ErrorResponse{Error: errMsg})
		return
	}

	d.mu.Lock()
	d.startedAt = time.Now()
	d.activeProfile = profile
	d.socksAddr = socksAddr
	d.httpAddr = req.HTTPAddr
	d.mixed = req.Mixed
	d.proxyAuth = req.ProxyAuth
	d.mode = mode
	d.tunName = tunName
	d.mu.Unlock()

	applog.Event("core started", "profile", profile, "mode", mode)
	writeJSON(w, http.StatusOK, d.statusSnapshot())
}

func (d *Daemon) handleStop(w http.ResponseWriter, _ *http.Request) {
	if err := d.runner.Stop(); err != nil {
		writeJSON(w, http.StatusInternalServerError, ipc.ErrorResponse{Error: err.Error()})
		return
	}
	applog.Event("core stopped")
	writeJSON(w, http.StatusOK, d.statusSnapshot())
}

// handleQuit завершает работу самого демона: клиент сможет поднять его заново,
// например с правами администратора.
func (d *Daemon) handleQuit(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, d.statusSnapshot())
	go func() {
		time.Sleep(150 * time.Millisecond) // дать ответу уйти клиенту
		d.quitOnce.Do(func() { close(d.quitCh) })
	}()
}

func (d *Daemon) handleLogs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, ipc.LogsResponse{Lines: d.runner.Logs()})
}

func (d *Daemon) handleLogsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming не поддерживается", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 128)
	d.subsMu.Lock()
	d.subs[ch] = struct{}{}
	d.subsMu.Unlock()

	defer func() {
		d.subsMu.Lock()
		delete(d.subs, ch)
		d.subsMu.Unlock()
	}()

	for _, line := range d.runner.Logs() {
		fmt.Fprintf(w, "data: %s\n\n", line)
	}
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-d.stopCh:
			return
		case line := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
		}
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// --- Меню разработчика ---

// devPatchedConfig возвращает конфиг с loglevel=info, если включён
// dev-режим логов; иначе nil.
func (d *Daemon) devPatchedConfig(cfg []byte) []byte {
	d.mu.Lock()
	on := d.devLog
	d.mu.Unlock()
	if !on {
		return nil
	}
	return patchLogLevel(cfg, "info")
}

// patchLogLevel перезаписывает log.loglevel в конфиге ядра. Xray не умеет
// менять уровень на лету, поэтому dev-переключение включает рестарт ядра.
func patchLogLevel(cfg []byte, level string) []byte {
	var doc map[string]any
	if err := json.Unmarshal(cfg, &doc); err != nil {
		return cfg
	}
	logBlock, ok := doc["log"].(map[string]any)
	if !ok {
		logBlock = map[string]any{}
		doc["log"] = logBlock
	}
	logBlock["loglevel"] = level
	out, err := json.Marshal(doc)
	if err != nil {
		return cfg
	}
	return out
}

// setDevLog включает/выключает зеркалирование логов ядра в файл и
// перезапускает ядро, чтобы новый уровень логов вступил в силу.
func (d *Daemon) setDevLog(on bool) error {
	d.mu.Lock()
	d.devLog = on
	if on {
		if err := os.MkdirAll(applog.Dir(), 0o700); err != nil {
			d.devLog = false
			d.mu.Unlock()
			return fmt.Errorf("не удалось создать каталог журналов: %w", err)
		}
		f, err := os.OpenFile(filepath.Join(applog.Dir(), "xray.log"),
			os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			d.devLog = false
			d.mu.Unlock()
			return fmt.Errorf("не удалось открыть зеркало логов: %w", err)
		}
		if d.logMirror != nil {
			_ = d.logMirror.Close()
		}
		d.logMirror = f
	} else if d.logMirror != nil {
		f := d.logMirror
		d.logMirror = nil
		_ = f.Close()
	}
	restart := d.runner.IsRunning()
	orig := append([]byte(nil), d.lastConfig...)
	assetDir := d.lastAssetDir
	d.mu.Unlock()

	applog.Event("dev kernel log mode", "on", on)
	if restart && len(orig) > 0 {
		cfg := orig
		if on {
			cfg = patchLogLevel(orig, "info")
		}
		_ = d.runner.Stop()
		if err := d.runner.Start(cfg, assetDir); err != nil {
			applog.Fail("core restart for dev log failed", err)
			return err
		}
		d.mu.Lock()
		d.startedAt = time.Now()
		d.mu.Unlock()
	}
	return nil
}

func (d *Daemon) handleDevLog(w http.ResponseWriter, r *http.Request) {
	var req ipc.DevLogRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ipc.ErrorResponse{Error: "невалидный JSON запроса"})
		return
	}
	if err := d.setDevLog(req.On); err != nil {
		writeJSON(w, http.StatusInternalServerError, ipc.ErrorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, d.statusSnapshot())
}

func (d *Daemon) handleDevStatus(w http.ResponseWriter, _ *http.Request) {
	d.mu.Lock()
	snap := ipc.DevStatusResponse{
		Running:   d.runner.IsRunning(),
		Profile:   d.activeProfile,
		Mode:      d.mode,
		DevLog:    d.devLog,
	}
	if !d.startedAt.IsZero() {
		snap.StartedAt = d.startedAt.Format(time.RFC3339)
		snap.UptimeSec = int64(time.Since(d.startedAt).Seconds())
	}
	d.mu.Unlock()
	if snap.Running {
		snap.PID, _ = d.runner.Pid()
		snap.Version = d.xrayVersion()
	}
	writeJSON(w, http.StatusOK, snap)
}

// xrayVersion определяет и кэширует версию ядра (один запуск на процесс).
func (d *Daemon) xrayVersion() string {
	d.mu.Lock()
	if d.version != "" {
		v := d.version
		d.mu.Unlock()
		return v
	}
	d.mu.Unlock()

	verCmd := exec.Command(d.xrayBin, "version")
	osutil.HideChildConsole(verCmd)
	out, err := verCmd.CombinedOutput()
	v := "unknown"
	if err == nil {
		if line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]; line != "" {
			v = line
		}
	}
	d.mu.Lock()
	d.version = v
	d.mu.Unlock()
	return v
}

func (d *Daemon) handleDevConfig(w http.ResponseWriter, _ *http.Request) {
	d.mu.Lock()
	cfg := append([]byte(nil), d.lastConfig...)
	d.mu.Unlock()

	if len(cfg) == 0 {
		writeJSON(w, http.StatusConflict, ipc.ErrorResponse{Error: "ядро ещё не запускалось — дамп нечего показать"})
		return
	}
	writeJSON(w, http.StatusOK, ipc.DevConfigResponse{Config: string(maskConfigSecrets(cfg))})
}

// maskConfigSecrets маскирует чувствительные значения (UUID/пароли/ключи)
// в дампе конфига: value[:4] + "…masked".
func maskConfigSecrets(cfg []byte) []byte {
	var doc any
	if err := json.Unmarshal(cfg, &doc); err != nil {
		return cfg
	}
	masked := maskNode(doc)
	out, err := json.MarshalIndent(masked, "", "  ")
	if err != nil {
		return cfg
	}
	return out
}

func maskNode(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			switch k {
			case "id", "password", "privateKey", "password1", "secret", "token":
				if s, ok := val.(string); ok && len(s) > 4 {
					t[k] = s[:4] + "…masked"
					continue
				}
			}
			t[k] = maskNode(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = maskNode(val)
		}
		return t
	default:
		return v
	}
}

func (d *Daemon) handleDevCheck(w http.ResponseWriter, _ *http.Request) {
	res := netcheck.Check()
	applog.Event("tunnel check", "tunnel_ip", res.TunnelIP, "direct_ip", res.DirectIP, "resolver", res.ResolverIP)
	writeJSON(w, http.StatusOK, ipc.DevCheckResponse{
		TunnelIP:    res.TunnelIP,
		DirectIP:    res.DirectIP,
		DirectIface: res.DirectIface,
		ResolverIP:  res.ResolverIP,
		Notes:       res.Notes,
	})
}
