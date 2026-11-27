package config

import "testing"

// Новые установки: IPv6 в TUN включён по умолчанию, уровень логов error.
func TestApplyDefaultsTUNIPv6AndLogLevel(t *testing.T) {
	s := &Store{}
	s.applyDefaults()

	if !s.Settings.TUNIPv6Value() {
		t.Fatal("IPv6 в TUN должен быть включён по умолчанию")
	}
	if s.Settings.LogLevel != "error" {
		t.Fatalf("LogLevel = %q, ждали error", s.Settings.LogLevel)
	}
}

// Явное «выкл» из старого конфига (tun_ipv6: false) не перезаписывается.
func TestApplyDefaultsKeepsExplicitIPv6Off(t *testing.T) {
	off := false
	s := &Store{Settings: Settings{TUNIPv6: &off}}
	s.applyDefaults()

	if s.Settings.TUNIPv6Value() {
		t.Fatal("явное tun_ipv6: false не должно перезаписываться дефолтом")
	}
}
