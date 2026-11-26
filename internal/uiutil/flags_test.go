package uiutil

import "testing"

func TestReplaceRegionalFlags(t *testing.T) {
	cases := []struct{ in, want string }{
		{"🇱🇹 Литва", "[LT] Литва"},
		{"Сервер 🇺🇸 США", "Сервер [US] США"},
		{"🇷🇺🇩🇪 два флага", "[RU][DE] два флага"},
		{"Огонёк 🔥 и ⚡ без флагов", "Огонёк 🔥 и ⚡ без флагов"},
		{"одинокий 🇦 индикатор", "одинокий  индикатор"},
		{"", ""},
	}
	for _, c := range cases {
		if got := replaceRegionalFlags(c.in); got != c.want {
			t.Errorf("replaceRegionalFlags(%q) = %q, ждали %q", c.in, got, c.want)
		}
	}
}

func TestReplaceRegionalFlagsKeepsPlainText(t *testing.T) {
	// Строки без индикаторов не должны копироваться зря и портиться.
	plain := "LT Литва, US United States"
	if got := replaceRegionalFlags(plain); got != plain {
		t.Fatalf("обычный текст изменён: %q", got)
	}
}
