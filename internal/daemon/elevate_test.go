package daemon

import (
	"context"
	"strings"
	"testing"

	"xray-tui/internal/osutil"
)

// На Unix запуск с повышением прав не поддерживается: клиент должен получить
// понятную ошибку, а не молчание.
func TestStartElevatedOnlyOnWindows(t *testing.T) {
	if osutil.TunRequiresAdmin() {
		t.Skip("проверка для не-Windows платформ")
	}
	err := StartElevated(context.Background(), "/tmp/fluffify-test.sock")
	if err == nil {
		t.Fatal("ждали ошибку на не-Windows платформе")
	}
	if !strings.Contains(err.Error(), "Windows") {
		t.Fatalf("неинформативная ошибка: %v", err)
	}
}

func TestRestartElevatedOnlyOnWindows(t *testing.T) {
	if osutil.TunRequiresAdmin() {
		t.Skip("проверка для не-Windows платформ")
	}
	if err := RestartElevated(context.Background(), "/tmp/fluffify-test.sock"); err == nil {
		t.Fatal("ждали ошибку на не-Windows платформе")
	}
}
