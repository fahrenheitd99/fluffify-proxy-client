package uiutil

import (
	"strings"
	"testing"
)

// Display — публичная точка входа: она используется и в TUI, и в трее,
// на обеих платформах, поэтому тесты идут через неё.
func TestDisplayReplacesFlags(t *testing.T) {
	cases := []struct{ in, want string }{
		{"🇱🇹 Литва", "[LT] Литва"},
		{"Сервер 🇺🇸 США", "Сервер [US] США"},
		{"🇷🇺🇩🇪 два флага", "[RU][DE] два флага"},
		{"Огонёк 🔥 и ⚡ без флагов", "Огонёк 🔥 и ⚡ без флагов"},
		{"одинокий 🇦 индикатор", "одинокий  индикатор"},
		{"", ""},
	}
	for _, c := range cases {
		if got := Display(c.in); got != c.want {
			t.Errorf("Display(%q) = %q, ждали %q", c.in, got, c.want)
		}
	}
}

func TestDisplayKeepsPlainText(t *testing.T) {
	// Строки без индикаторов не должны копироваться зря и портиться.
	plain := "LT Литва, US United States, vless://uuid@host:443?sni=a.b#node"
	if got := Display(plain); got != plain {
		t.Fatalf("обычный текст изменён: %q", got)
	}
}

// После Display в строке не должно оставаться региональных индикаторов —
// иначе Windows-терминал снова сломает высоту строки.
func TestDisplayLeavesNoIndicators(t *testing.T) {
	for _, s := range []string{"🇱🇹", "🇺🇸🇩🇪", "a🇷🇺b", "🇦", "🇱🇹🇺🇸🇩🇪🇷🇺"} {
		got := Display(s)
		if strings.ContainsFunc(got, isRegionalIndicator) {
			t.Errorf("Display(%q) = %q: остался региональный индикатор", s, got)
		}
	}
}

func TestReplaceRegionalFlags(t *testing.T) {
	if got := replaceRegionalFlags("🇱🇹"); got != "[LT]" {
		t.Fatalf("replaceRegionalFlags = %q", got)
	}
	if got := replaceRegionalFlags("нет флагов"); got != "нет флагов" {
		t.Fatalf("replaceRegionalFlags испортил текст: %q", got)
	}
}
