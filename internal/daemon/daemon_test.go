package daemon

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleStartTUNWithoutPermission(t *testing.T) {
	old := hasTunPermission
	hasTunPermission = func() bool { return false }
	defer func() { hasTunPermission = old }()

	d := New("/tmp/fluffify-test.sock")

	req := httptest.NewRequest(http.MethodPost, "/start",
		strings.NewReader(`{"mode":"tun","config":{"outbounds":[]}}`))
	rec := httptest.NewRecorder()
	d.handleStart(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("код ответа %d, ждали %d: %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "TUN") || !strings.Contains(body, "прав") {
		t.Fatalf("в ошибке нет подсказки про права: %s", body)
	}
}

func TestStatusReportsElevation(t *testing.T) {
	d := New("/tmp/fluffify-test.sock")
	d.elevated = true
	if !d.statusSnapshot().Elevated {
		t.Fatal("статус должен сообщать о правах администратора")
	}
}
