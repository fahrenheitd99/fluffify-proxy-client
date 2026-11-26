package daemon

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// В Windows TUN требует прав администратора у демона: без прав /start
// отвечает 409 с подсказкой перезапустить демон через restart-admin.
func TestHandleStartTUNWithoutRights(t *testing.T) {
	old := tunRequiresAdmin
	tunRequiresAdmin = func() bool { return true }
	defer func() { tunRequiresAdmin = old }()

	d := New("/tmp/fluffify-test.sock", "")
	d.elevated = false

	req := httptest.NewRequest(http.MethodPost, "/start",
		strings.NewReader(`{"mode":"tun","config":{"outbounds":[]}}`))
	rec := httptest.NewRecorder()
	d.handleStart(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("код ответа %d, ждали %d: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "TUN") || !strings.Contains(body, "restart-admin") {
		t.Fatalf("в ошибке нет подсказки про restart-admin: %s", body)
	}
}

// Регрессия: ошибка ядра в TUN-режиме у демона С правами не должна
// маскироваться под проблему отсутствия прав.
func TestHandleStartTUNCoreErrorIsNotRightsHint(t *testing.T) {
	old := tunRequiresAdmin
	tunRequiresAdmin = func() bool { return true }
	defer func() { tunRequiresAdmin = old }()

	d := New("/tmp/fluffify-test.sock", "")
	d.elevated = true

	// Занимаем порт: старт ядра с этим портом упадёт с ошибкой окружения.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("не удалось занять порт: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	cfg := fmt.Sprintf(`{"mode":"tun","config":{"inbounds":[{"tag":"i","listen":"127.0.0.1","port":%d,"protocol":"socks"}],"outbounds":[{"tag":"direct","protocol":"freedom"}]}}`, port)
	req := httptest.NewRequest(http.MethodPost, "/start", strings.NewReader(cfg))
	rec := httptest.NewRecorder()
	d.handleStart(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatal("ожидаем ошибку: порт уже занят")
	}
	body := rec.Body.String()
	if strings.Contains(strings.ToLower(body), "администратор") {
		t.Fatalf("ошибка ядра не должна выдаваться за проблему прав: %s", body)
	}
}

func TestStatusReportsElevation(t *testing.T) {
	d := New("/tmp/fluffify-test.sock", "")
	d.elevated = true
	if !d.statusSnapshot().Elevated {
		t.Fatal("статус должен сообщать о правах администратора")
	}
}
