package generator

import (
	"strings"
	"testing"

	"xray-tui/internal/config"
)

func TestRoutingNeedsGeoDocs(t *testing.T) {
	builtin := &config.RoutingProfile{
		Rules: []config.RoutingRule{
			{Name: "private", Domains: []string{"geosite:private"}, IPs: []string{"geoip:private"}},
			{Name: "direct", Domains: []string{"example.com", "regexp:.*\\.local$"}},
		},
	}
	if routingNeedsGeoDocs(builtin) {
		t.Fatal("встроенные категории private не требуют геобаз")
	}

	needsDocs := &config.RoutingProfile{
		Rules: []config.RoutingRule{{Name: "cn", Domains: []string{"geosite:cn"}}},
	}
	if !routingNeedsGeoDocs(needsDocs) {
		t.Fatal("geosite:cn требует файл геобаз")
	}

	ips := &config.RoutingProfile{
		Rules: []config.RoutingRule{{Name: "ru", IPs: []string{"geoip:ru"}}},
	}
	if !routingNeedsGeoDocs(ips) {
		t.Fatal("geoip:ru требует файл геобаз")
	}
}

// Без скачанных геобаз сборка конфига с geosite:cn должна падать понятно,
// а со встроенной категорией private — работать (файлы не нужны).
func TestBuildForProfileGeoDocsRequirement(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	store, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	prof := config.Profile{
		ID:   "p1",
		Name: "node",
		Type: config.ProfileTypeURI,
		URI:  "vless://11111111-1111-1111-1111-111111111111@a.example:443",
	}
	store.Profiles = []config.Profile{prof}
	store.Routing.Enabled = true
	store.Routing.Profiles = []config.RoutingProfile{{
		ID:            "r1",
		Name:          "custom",
		GeoProviderID: config.DefaultGeoProviderID,
		Rules: []config.RoutingRule{
			{Name: "cn", Domains: []string{"geosite:cn"}, Outbound: "proxy", Enabled: true},
		},
	}}
	store.Routing.ActiveProfileID = "r1"

	if _, err := BuildForProfile(store, &store.Profiles[0]); err == nil ||
		!strings.Contains(err.Error(), "геобазы") {
		t.Fatalf("ждали ошибку про геобазы, получили %v", err)
	}

	store.Routing.Profiles[0].Rules = []config.RoutingRule{
		{Name: "private", Domains: []string{"geosite:private"}, IPs: []string{"geoip:private"}, Outbound: "direct", Enabled: true},
	}
	if _, err := BuildForProfile(store, &store.Profiles[0]); err != nil {
		t.Fatalf("встроенная категория private не должна требовать геобаз: %v", err)
	}
}
