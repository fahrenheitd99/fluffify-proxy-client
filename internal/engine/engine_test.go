package engine

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// freePort занимает и сразу освобождает порт, чтобы отдать его ядру.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("не удалось занять порт: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func socksConfig(port int, level string) []byte {
	return []byte(fmt.Sprintf(`{
  "log": {"loglevel": %q},
  "inbounds": [{
    "tag": "socks",
    "listen": "127.0.0.1",
    "port": %d,
    "protocol": "socks",
    "settings": {"udp": false}
  }],
  "outbounds": [{"tag": "direct", "protocol": "freedom"}]
}`, level, port))
}

// Дымовой тест: ядро поднимается в памяти, слушает порт, отдаёт логи
// в колбэк и корректно освобождает порт при остановке.
func TestEngineStartsInProcess(t *testing.T) {
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	var mu sync.Mutex
	var lines []string
	eng := New(Options{
		OnLog: func(line string) {
			mu.Lock()
			lines = append(lines, line)
			mu.Unlock()
		},
	})

	if err := eng.Start(socksConfig(port, "info"), ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = eng.Stop() }()

	if !eng.Running() {
		t.Fatal("после Start движок должен считаться запущенным")
	}

	// SOCKS5-приветствие доказывает, что инбаунд реально работает в процессе.
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		t.Fatalf("инбаунд не слушает %s: %v", addr, err)
	}
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		t.Fatalf("не удалось отправить SOCKS-приветствие: %v", err)
	}
	reply := make([]byte, 2)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Read(reply); err != nil {
		t.Fatalf("нет ответа SOCKS: %v", err)
	}
	_ = conn.Close()
	if reply[0] != 0x05 || reply[1] != 0x00 {
		t.Fatalf("неожиданный ответ SOCKS: %v", reply)
	}

	// Логи ядра должны доходить до колбэка (их показывает панель логов).
	deadline := time.Now().Add(2 * time.Second)
	var got []string
	for time.Now().Before(deadline) {
		mu.Lock()
		got = append([]string(nil), lines...)
		mu.Unlock()
		if len(got) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(got) == 0 {
		t.Fatal("ядро не передало ни одной строки лога")
	}
	if !strings.Contains(strings.Join(got, "\n"), "started") {
		t.Logf("лог ядра: %v", got)
	}

	if err := eng.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if eng.Running() {
		t.Fatal("после Stop движок должен быть остановлен")
	}

	if conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatalf("порт %s всё ещё слушается после Stop", addr)
	}
}

// Перезапуск на лету: смена конфига — это Stop + Start, без внешних процессов.
func TestEngineRestartsWithNewConfig(t *testing.T) {
	portA := freePort(t)
	portB := freePort(t)

	eng := New(Options{})
	if err := eng.Start(socksConfig(portA, "warning"), ""); err != nil {
		t.Fatalf("первый Start: %v", err)
	}
	if err := eng.Start(socksConfig(portB, "warning"), ""); err != nil {
		t.Fatalf("второй Start: %v", err)
	}
	defer func() { _ = eng.Stop() }()

	if conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", portB), 3*time.Second); err != nil {
		t.Fatalf("новый порт не слушается: %v", err)
	} else {
		_ = conn.Close()
	}
	if conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", portA), 500*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatal("старый порт остался занят после смены конфига")
	}
}

func TestEngineRejectsBrokenConfig(t *testing.T) {
	eng := New(Options{})
	defer func() { _ = eng.Stop() }()

	if err := eng.Start([]byte("{не json"), ""); err == nil {
		t.Fatal("сломанный конфиг должен возвращать ошибку")
	}
	if eng.Running() {
		t.Fatal("после ошибки движок не должен считаться запущенным")
	}
	if err := eng.Start(nil, ""); err == nil {
		t.Fatal("пустой конфиг должен возвращать ошибку")
	}
}

func TestHasTunInbound(t *testing.T) {
	if !hasTunInbound([]byte(`{"inbounds":[{"protocol":"tun"}]}`)) {
		t.Fatal("TUN-конфиг должен распознаваться")
	}
	if hasTunInbound([]byte(`{"inbounds":[{"protocol":"socks"}]}`)) {
		t.Fatal("конфиг без TUN не должен распознаваться как TUN")
	}
}
