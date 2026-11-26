package about

import (
	"strings"
	"testing"
)

// Лицензии обязаны быть встроены: без них распространять exe с ядром Xray
// и драйвером wintun нельзя.
func TestEntriesEmbedded(t *testing.T) {
	entries := Entries()
	if len(entries) != 2 {
		t.Fatalf("записей лицензий %d, ждали 2", len(entries))
	}
	var xray, wintun bool
	for _, e := range entries {
		if strings.TrimSpace(e.Text) == "" {
			t.Fatalf("пустой текст лицензии %q", e.Name)
		}
		switch {
		case strings.Contains(e.Name, "Xray-core"):
			xray = true
			if !strings.Contains(e.Text, "Mozilla Public License") {
				t.Errorf("лицензия Xray без MPL-2.0: %q", e.Name)
			}
		case strings.Contains(e.Name, "wintun"):
			wintun = true
		}
	}
	if !xray || !wintun {
		t.Fatalf("нет лицензий: xray=%v wintun=%v", xray, wintun)
	}
}

func TestCombined(t *testing.T) {
	combined := Combined()
	if !strings.Contains(combined, "=== Xray-core") || !strings.Contains(combined, "=== wintun") {
		t.Fatalf("Combined не содержит оба заголовка")
	}
	if strings.Contains(combined, "\n\n\n") {
		t.Fatal("лишние пустые строки в Combined")
	}
}
