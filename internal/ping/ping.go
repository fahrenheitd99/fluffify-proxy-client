package ping

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"xray-tui/internal/osutil"
)

const DefaultTestURL = "https://cp.cloudflare.com/generate_204"

// Measure поднимает временный xray и замеряет HTTP HEAD RTT через прокси.
func Measure(ctx context.Context, xrayBin string, rawConfig []byte, testURL string) (time.Duration, error) {
	if xrayBin == "" {
		xrayBin = osutil.ResolveBinary("xray")
	}
	if testURL == "" {
		testURL = DefaultTestURL
	}

	port, err := findFreePort()
	if err != nil {
		return 0, fmt.Errorf("не удалось найти свободный порт: %w", err)
	}

	patchedCfg, err := preparePingConfig(rawConfig, port)
	if err != nil {
		return 0, err
	}

	runCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, xrayBin, "run", "-c", "stdin:")
	cmd.Stdin = bytes.NewReader(patchedCfg)
	if dir := osutil.SiblingAssetDir(xrayBin); dir != "" {
		cmd.Env = append(os.Environ(), "XRAY_LOCATION_ASSET="+dir)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("ошибка запуска xray: %w", err)
	}

	exitCh := make(chan error, 1)
	go func() {
		exitCh <- cmd.Wait()
	}()

	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-exitCh
	}()

	proxyAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	if err := waitPort(runCtx, proxyAddr, exitCh, &stderr); err != nil {
		return 0, err
	}

	proxyURL, _ := url.Parse("http://" + proxyAddr)
	transport := &http.Transport{
		Proxy:             http.ProxyURL(proxyURL),
		DisableKeepAlives: true,
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}

	req, err := http.NewRequestWithContext(runCtx, http.MethodHead, testURL, nil)
	if err != nil {
		return 0, err
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("нет ответа: %w", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return time.Since(start), nil
}

// MeasureTCP замеряет прямое время TCP-подключения к адресу и порту сервера.
// MeasureTCP сначала отдельно резолвит DNS, затем замеряет чистое время TCP-рукопожатия.
func MeasureTCP(ctx context.Context, host string, port int) (time.Duration, error) {
	if host == "" || port <= 0 {
		return 0, errors.New("не указан адрес или порт сервера")
	}

	targetIP := host
	if net.ParseIP(host) == nil {
		resolveCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		addrs, err := net.DefaultResolver.LookupIPAddr(resolveCtx, host)
		cancel()
		if err != nil || len(addrs) == 0 {
			return 0, fmt.Errorf("DNS не ответил для %s", host)
		}
		targetIP = addrs[0].IP.String()
		for _, a := range addrs {
			if a.IP.To4() != nil {
				targetIP = a.IP.String()
				break
			}
		}
	}

	addr := net.JoinHostPort(targetIP, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: 3 * time.Second}

	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, err
	}
	rtt := time.Since(start)
	_ = conn.Close()
	return rtt, nil
}

// ExtractEndpoint пытается достать address и port первого прокси-outbound из Raw JSON.
// ExtractEndpoint находит реальный входной address:port в Raw JSON (с учётом цепочек dialerProxy).
func ExtractEndpoint(raw []byte) (string, int, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", 0, err
	}
	outbounds, _ := doc["outbounds"].([]any)
	byTag := make(map[string]map[string]any, len(outbounds))
	var first map[string]any

	for _, item := range outbounds {
		ob, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if tag, _ := ob["tag"].(string); tag != "" {
			byTag[tag] = ob
		}
		proto, _ := ob["protocol"].(string)
		if first == nil && proto != "freedom" && proto != "blackhole" && proto != "dns" {
			first = ob
		}
	}
	if first == nil {
		return "", 0, errors.New("в Raw JSON не найден прокси-outbound")
	}

	// Идём по цепочке dialerProxy до реальной входной точки
	cur := first
	for depth := 0; depth < 5; depth++ {
		stream, _ := cur["streamSettings"].(map[string]any)
		sockopt, _ := stream["sockopt"].(map[string]any)
		nextTag, _ := sockopt["dialerProxy"].(string)
		if nextTag == "" {
			break
		}
		nextOb, ok := byTag[nextTag]
		if !ok {
			break
		}
		nextProto, _ := nextOb["protocol"].(string)
		if nextProto == "freedom" || nextProto == "blackhole" || nextProto == "dns" {
			break
		}
		cur = nextOb
	}

	settings, _ := cur["settings"].(map[string]any)
	if settings != nil {
		if a, _ := settings["address"].(string); a != "" {
			if p, ok := settings["port"].(float64); ok && p > 0 {
				return a, int(p), nil
			}
		}
		if vnext, ok := settings["vnext"].([]any); ok && len(vnext) > 0 {
			if vn, ok := vnext[0].(map[string]any); ok {
				a, _ := vn["address"].(string)
				p, _ := vn["port"].(float64)
				if a != "" && p > 0 {
					return a, int(p), nil
				}
			}
		}
	}
	return "", 0, errors.New("не удалось определить адрес сервера из Raw JSON")
}

func preparePingConfig(raw []byte, port int) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("невалидный JSON конфиг: %w", err)
	}

	delete(doc, "api")
	delete(doc, "stats")

	if routing, ok := doc["routing"].(map[string]any); ok {
		if rules, ok := routing["rules"].([]any); ok {
			filtered := make([]any, 0, len(rules))
			for _, r := range rules {
				m, ok := r.(map[string]any)
				if ok {
					if outTag, _ := m["outboundTag"].(string); outTag == "api" {
						continue
					}
				}
				filtered = append(filtered, r)
			}
			routing["rules"] = filtered
		}
	}

	doc["log"] = map[string]any{"loglevel": "error"}
	doc["inbounds"] = []map[string]any{
		{
			"tag":      "ping-http",
			"listen":   "127.0.0.1",
			"port":     port,
			"protocol": "http",
		},
	}

	return json.Marshal(doc)
}

func findFreePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func waitPort(ctx context.Context, addr string, exitCh <-chan error, stderr *bytes.Buffer) error {
	deadline := time.NewTimer(2500 * time.Millisecond)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-exitCh:
			msg := strings.TrimSpace(stderr.String())
			if msg != "" {
				lines := strings.Split(msg, "\n")
				return errors.New(lines[len(lines)-1])
			}
			return fmt.Errorf("xray завершился до старта: %v", err)
		case <-deadline.C:
			return errors.New("таймаут запуска временного xray")
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				return nil
			}
		}
	}
}
