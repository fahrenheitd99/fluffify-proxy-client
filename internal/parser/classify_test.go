package parser

import "testing"

func TestClassifyInput(t *testing.T) {
	cases := []struct {
		name string
		in   string
		kind InputKind
	}{
		{"подписка", "https://sub.example.com/abc", InputSubscription},
		{"подписка с кавычками", `"https://sub.example.com/abc"`, InputSubscription},
		{"подписка с маркерами вставки", "\x1b[200~https://sub.example/x\x1b[201~", InputSubscription},
		{"подписка url-encoded", "%68%74%74%70%73%3A%2F%2Fsub.example%2Fx", InputSubscription},
		{"vless", "vless://uuid@host:443?type=tcp#node", InputLink},
		{"hysteria2", "hysteria2://pass@host:443/?sni=x", InputLink},
		{"vmess", "vmess://eyJ2IjoiMiJ9", InputLink},
		{"vless в маркерах вставки", "\x1b[200~vless://uuid@host:443\x1b[201~", InputLink},
		{"json объект", `{"outbounds":[{"protocol":"vless"}]}`, InputJSON},
		{"json массив", `[{"domain":"x.y"}]`, InputJSON},
		{"путь к файлу", `/home/user/node.json`, InputFilePath},
		{"cidr-мусор из url-encoded", "%B7%F1%F2%F0%EE%EA%E0", InputFilePath},
	}
	for _, c := range cases {
		kind, value := ClassifyInput(c.in)
		if kind != c.kind {
			t.Errorf("%s: ClassifyInput(%q) = %v, ждали %v", c.name, c.in, kind, c.kind)
		}
		if value == "" {
			t.Errorf("%s: пустое значение после нормализации", c.name)
		}
	}
}

func TestNormalizeInput(t *testing.T) {
	if got := NormalizeInput("  \"vless://a@b:1\"  "); got != "vless://a@b:1" {
		t.Fatalf("кавычки не сняты: %q", got)
	}
	if got := NormalizeInput("%68%74%74%70%73%3A%2F%2Fsub%2Fx"); got != "https://sub/x" {
		t.Fatalf("url-декодирование не сработало: %q", got)
	}
	// Адрес со схемой не перекодируем: у ключей %XX живёт в параметрах.
	if got := NormalizeInput("https://sub%2Fx"); got != "https://sub%2Fx" {
		t.Fatalf("готовый URL не должен декодироваться: %q", got)
	}
	link := "vless://u@h:443?path=%2Fws&security=tls"
	if got := NormalizeInput(link); got != link {
		t.Fatalf("ссылку трогать нельзя: %q", got)
	}
	if got := NormalizeInput("\x1b[200~\x1b[201~"); got != "" {
		t.Fatalf("маркеры должны исчезнуть: %q", got)
	}
}

func TestTotalLines(t *testing.T) {
	got := TotalLines("vless://a@b:1\n\n hysteria2://c@d:2 \r\n")
	if len(got) != 2 || got[0] != "vless://a@b:1" || got[1] != "hysteria2://c@d:2" {
		t.Fatalf("TotalLines = %#v", got)
	}
}
