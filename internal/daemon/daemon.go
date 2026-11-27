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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"xray-tui/internal/config"
	"xray-tui/internal/core"
	"xray-tui/internal/generator"
	"xray-tui/internal/ipc"
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
		subs:       make(map[chan string]struct{}),
		stopCh:     make(chan struct{}),
		quitCh:     make(chan struct{}),
		elevated:   osutil.IsElevated(),
	}
	d.runner = core.NewRunner(xrayBin, d.broadcastLog)
	return d
}

func (d *Daemon) Run(ctx context.Context) error {
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

	if err := d.runner.Start(res.Config, res.AssetDir); err != nil {
		fmt.Fprintf(os.Stderr, "[daemon] ошибка автоподключения: %v\n", err)
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
}

func (d *Daemon) broadcastLog(line string) {
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

	if d.runner.IsRunning() {
		if err := d.runner.Stop(); err != nil {
			writeJSON(w, http.StatusInternalServerError, ipc.ErrorResponse{Error: err.Error()})
			return
		}
	}

	if err := d.runner.Start(cfg, req.AssetDir); err != nil {
		writeJSON(w, http.StatusConflict, ipc.ErrorResponse{Error: err.Error()})
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
			// Нода /dev/net/tun есть всегда, а вот модуль ядра tun часто не
			// загружен — тогда никакие права не помогут, и подсказка про
			// tun-setup вводила бы в заблуждение.
			if !osutil.TUNDeviceReady() {
				errMsg = "TUN не запустился — модуль ядра tun не загружен: выполни " +
					"sudo modprobe tun (или " + osutil.SelfName() + " tun-setup)"
			} else {
				errMsg = "TUN не запустился — выполни: " + osutil.SelfName() + " tun-setup"
			}
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

	writeJSON(w, http.StatusOK, d.statusSnapshot())
}

func (d *Daemon) handleStop(w http.ResponseWriter, _ *http.Request) {
	if err := d.runner.Stop(); err != nil {
		writeJSON(w, http.StatusInternalServerError, ipc.ErrorResponse{Error: err.Error()})
		return
	}
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
