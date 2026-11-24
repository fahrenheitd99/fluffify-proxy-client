package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	ProfileTypeURI = "uri"
	ProfileTypeRaw = "raw"

	ModeProxy = "proxy"
	ModeTUN   = "tun"
	UIModeKeys    = "keys"
	UIModeButtons = "buttons"

	PingModeProxyHEAD = "proxy_head"
	PingModeTCP       = "tcping"
	PingModeICMP 	  = "icmp"

	DefaultGeoProviderID = "fahrenheitd99"
	DefaultPingURL       = "https://cp.cloudflare.com/generate_204"
    
    ThemeSystem       = "system"
	ThemeDefaultDark  = "default_dark"
	ThemeDefaultLight = "default_light"
	ThemeGruvbox      = "gruvbox"
	ThemeOwo          = "owo"
	ThemeUwu 		  = "uwu"
	ThemeGTAVI        = "gta_vi"
	ThemeHackerBlue  = "hacker_blue"
	ThemeHackerGreen = "hacker_green"
	ThemeHackerRed   = "hacker_red"
	ThemeAmberCRT    = "amber_crt"
	ThemePhosphorCRT = "phosphor_crt"
	ThemeMatchaDark  = "matcha_dark"
	ThemeMatchaLight = "matcha_light"
	ThemeLavender    = "lavender"
	ThemePeach       = "peach"
	ThemePaleLavender = "pale_lavender"

	AccentRed    = "red"
	AccentOrange = "orange"
	AccentYellow = "yellow"
	AccentGreen  = "green"
	AccentCyan   = "cyan"
	AccentBlue   = "blue"
	AccentViolet = "violet"
	AccentPink  = "pink"
	AccentBrown = "brown"
	AccentWhite = "white"
	AccentBlack = "black"
)

var PresetPingURLs = []string{
	"https://cp.cloudflare.com/generate_204",
	"https://www.google.com/generate_204",
	"https://www.gstatic.com/generate_204",
}

type Settings struct {
	Listen          string `json:"listen"`
	SocksPort       int    `json:"socks_port"`
	HTTPPort        *int   `json:"http_port,omitempty"`
	MixedInbound    bool   `json:"mixed_inbound"`
	Sniffing        *bool  `json:"sniffing,omitempty"`
	ProxyAuth       bool   `json:"proxy_auth"`
	ProxyUser       string `json:"proxy_user,omitempty"`
	ProxyPass       string `json:"proxy_pass,omitempty"`
	LogLevel        string `json:"log_level"`
	HWID            string `json:"hwid"`
	DefaultUA       string `json:"default_ua"`
	ActiveProfileID string `json:"active_profile_id,omitempty"`
	WindowCols      int    `json:"window_cols"`
	WindowLines     int    `json:"window_lines"`
	Terminal        string `json:"terminal,omitempty"`

	Mode         string `json:"mode"`
	PingMode     string `json:"ping_mode"`
	PingURL      string `json:"ping_url"`
	AutoSortPing bool   `json:"auto_sort_ping"`
	AutoConnect  bool   `json:"auto_connect"`
	TUNInterface string `json:"tun_interface"`
	TUNMTU       int    `json:"tun_mtu"`
	TUNAutoDNS   bool   `json:"tun_auto_dns"`
	TUNIPv6      bool   `json:"tun_ipv6"`
	Theme        string `json:"theme"`
	AccentColor  string `json:"accent_color"`
	UIMode       string `json:"ui_mode,omitempty"`
}

func (s *Settings) SniffingEnabled() bool {
	return s.Sniffing == nil || *s.Sniffing
}

func (s *Settings) SetSniffing(v bool) {
	s.Sniffing = &v
}

func (s *Settings) GetHTTPPort() int {
	if s.HTTPPort == nil {
		return 10809
	}
	return *s.HTTPPort
}

func (s *Settings) SetHTTPPort(p int) {
	s.HTTPPort = &p
}

type GeoProvider struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	GeositeURL string `json:"geosite_url"`
	GeoipURL   string `json:"geoip_url"`
}

type RoutingRule struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Enabled    bool              `json:"enabled"`
	Domains    []string          `json:"domains,omitempty"`
	IPs        []string          `json:"ips,omitempty"`
	Port       string            `json:"port,omitempty"`
	SourcePort string            `json:"source_port,omitempty"`
	LocalPort  string            `json:"local_port,omitempty"`
	Network    string            `json:"network,omitempty"`
	Source     []string          `json:"source,omitempty"`
	LocalIP    []string          `json:"local_ip,omitempty"`
	Protocol   []string          `json:"protocol,omitempty"`
	InboundTag []string          `json:"inbound_tag,omitempty"`
	Attrs      map[string]string `json:"attrs,omitempty"`
	Outbound   string            `json:"outbound"` // "proxy", "direct", "block"
}

type RoutingProfile struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	DomainStrategy string        `json:"domain_strategy"`
	RawJSONMode    string        `json:"raw_json_mode"`
	GeoProviderID  string        `json:"geo_provider_id"`
	Rules          []RoutingRule `json:"rules"`
}

type RoutingConfig struct {
	Enabled         bool             `json:"enabled"`
	ActiveProfileID string           `json:"active_profile_id,omitempty"`
	GeoProviders    []GeoProvider    `json:"geo_providers"`
	Profiles        []RoutingProfile `json:"profiles"`
}

type SubInfo struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
	Expire   int64 `json:"expire"`
}

type Subscription struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	URL        string    `json:"url"`
	UserAgent  string    `json:"user_agent,omitempty"`
	SendHWID   bool      `json:"send_hwid"`
	CustomHWID string    `json:"custom_hwid,omitempty"`
	UpdatedAt  time.Time `json:"updated_at,omitempty"`
	Info       *SubInfo  `json:"info,omitempty"`
}

type Profile struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Protocol string          `json:"protocol"`
	Address  string          `json:"address,omitempty"`
	Port     int             `json:"port,omitempty"`
	URI      string          `json:"uri,omitempty"`
	RawJSON  json.RawMessage `json:"raw_json,omitempty"`
	SubID    string          `json:"sub_id,omitempty"`
	AddedAt  time.Time       `json:"added_at"`
}

type Store struct {
	Settings      Settings       `json:"settings"`
	Routing       RoutingConfig  `json:"routing"`
	Subscriptions []Subscription `json:"subscriptions"`
	Profiles      []Profile      `json:"profiles"`

	path string
}

func DefaultRoutingRules() []RoutingRule {
	return []RoutingRule{
		{
			ID:       NewID(),
			Name:     "Торренты",
			Enabled:  true,
			Protocol: []string{"bittorrent"},
			Outbound: "direct",
		},
		{
			ID:       NewID(),
			Name:     "Локальные IP",
			Enabled:  true,
			IPs:      []string{"geoip:private"},
			Outbound: "direct",
		},
		{
			ID:       NewID(),
			Name:     "Локальные домены",
			Enabled:  true,
			Domains:  []string{"geosite:private"},
			Outbound: "direct",
		},
		{
			ID:       NewID(),
			Name:     "Весь трафик IPv4",
			Enabled:  true,
			IPs:      []string{"0.0.0.0/0"},
			Outbound: "proxy",
		},
	}
}

func ConfigDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "xray-tui")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".xray-tui"
	}
	return filepath.Join(home, ".config", "xray-tui")
}

func GeoBaseDir() string {
	if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
		return filepath.Join(dir, "xray-tui", "geo")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".xray-tui-geo"
	}
	return filepath.Join(home, ".local", "share", "xray-tui", "geo")
}

func GeoProviderDir(providerID string) string {
	if providerID == "" {
		providerID = DefaultGeoProviderID
	}
	return filepath.Join(GeoBaseDir(), providerID)
}

func StorePath() string {
	return filepath.Join(ConfigDir(), "store.json")
}

func Load() (*Store, error) {
	path := StorePath()
	s := &Store{path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("не удалось прочитать %s: %w", path, err)
		}
		s.applyDefaults()
		return s, nil
	}

	if len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, s); err != nil {
			return nil, fmt.Errorf("повреждён файл %s: %w", path, err)
		}
	}
	s.applyDefaults()
	return s, nil
}

func (s *Store) applyDefaults() {
	if s.Settings.Listen == "" {
		s.Settings.Listen = "127.0.0.1"
	}
	if s.Settings.SocksPort == 0 {
		s.Settings.SocksPort = 10808
	}
	if s.Settings.HTTPPort == nil {
		p := 10809
		s.Settings.HTTPPort = &p
	}
	if s.Settings.Sniffing == nil {
		t := true
		s.Settings.Sniffing = &t
	}
	if s.Settings.LogLevel == "" {
		s.Settings.LogLevel = "warning"
	}
	if s.Settings.DefaultUA == "" {
		s.Settings.DefaultUA = "Fluffify/1.0.0"
	}
	if s.Settings.HWID == "" {
		s.Settings.HWID = GenerateHWID()
	}
	if s.Settings.WindowCols == 0 {
		s.Settings.WindowCols = 48
	}
	if s.Settings.WindowLines == 0 {
		s.Settings.WindowLines = 34
	}
	if s.Settings.Mode == "" {
		s.Settings.Mode = ModeProxy
	}
	if s.Settings.PingMode == "" {
		s.Settings.PingMode = PingModeProxyHEAD
	}
	if s.Settings.PingURL == "" {
		s.Settings.PingURL = DefaultPingURL
	}
	if s.Settings.TUNInterface == "" {
		s.Settings.TUNInterface = "xray0"
	}
	if s.Settings.TUNMTU == 0 {
		s.Settings.TUNMTU = 1500
	}
	if s.Settings.LogLevel == "" {
		s.Settings.LogLevel = "warning"
	}

	if len(s.Routing.GeoProviders) == 0 {
		s.Routing.GeoProviders = []GeoProvider{
			{
				ID:         DefaultGeoProviderID,
				Name:       "fahrenheitd99",
				GeositeURL: "https://raw.githubusercontent.com/fahrenheitd99/iplist-geofiles/main/geosite.dat",
				GeoipURL:   "https://raw.githubusercontent.com/fahrenheitd99/iplist-geofiles/main/geoip.dat",
			},
		}
	}

	if len(s.Routing.Profiles) == 0 {
		defProf := RoutingProfile{
			ID:             NewID(),
			Name:           "Основной",
			DomainStrategy: "IPIfNonMatch",
			RawJSONMode:    "prepend",
			GeoProviderID:  s.Routing.GeoProviders[0].ID,
			Rules:          DefaultRoutingRules(),
		}
		s.Routing.Profiles = []RoutingProfile{defProf}
		s.Routing.ActiveProfileID = defProf.ID
	}

	if s.ActiveRoutingProfile() == nil && len(s.Routing.Profiles) > 0 {
		s.Routing.ActiveProfileID = s.Routing.Profiles[0].ID
	}
	for i := range s.Routing.Profiles {
		if s.Routing.Profiles[i].DomainStrategy == "" {
			s.Routing.Profiles[i].DomainStrategy = "IPIfNonMatch"
		}
		if s.Routing.Profiles[i].RawJSONMode == "" {
			s.Routing.Profiles[i].RawJSONMode = "prepend"
		}
		if s.FindGeoProvider(s.Routing.Profiles[i].GeoProviderID) == nil && len(s.Routing.GeoProviders) > 0 {
			s.Routing.Profiles[i].GeoProviderID = s.Routing.GeoProviders[0].ID
		}
	}
	
	if s.Settings.Theme == "" {
		s.Settings.Theme = ThemeSystem
	}
	if s.Settings.AccentColor == "" {
		s.Settings.AccentColor = AccentCyan
	}
}

func (s *Store) ActiveRoutingProfile() *RoutingProfile {
	for i := range s.Routing.Profiles {
		if s.Routing.Profiles[i].ID == s.Routing.ActiveProfileID {
			return &s.Routing.Profiles[i]
		}
	}
	return nil
}

func (s *Store) FindGeoProvider(id string) *GeoProvider {
	for i := range s.Routing.GeoProviders {
		if s.Routing.GeoProviders[i].ID == id {
			return &s.Routing.GeoProviders[i]
		}
	}
	return nil
}

func (s *Store) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("не удалось создать %s: %w", filepath.Dir(s.path), err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) FindProfile(query string) (*Profile, int, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, -1, errors.New("не указан профиль")
	}

	if idx, err := strconv.Atoi(query); err == nil && idx >= 1 && idx <= len(s.Profiles) {
		return &s.Profiles[idx-1], idx - 1, nil
	}

	lower := strings.ToLower(query)
	for i := range s.Profiles {
		if s.Profiles[i].ID == query || strings.ToLower(s.Profiles[i].Name) == lower {
			return &s.Profiles[i], i, nil
		}
	}

	matches := make([]int, 0, 2)
	for i := range s.Profiles {
		if strings.Contains(strings.ToLower(s.Profiles[i].Name), lower) {
			matches = append(matches, i)
		}
	}
	if len(matches) == 1 {
		i := matches[0]
		return &s.Profiles[i], i, nil
	}
	if len(matches) > 1 {
		return nil, -1, fmt.Errorf("под %q подходит сразу %d профилей — укажи номер из 'xray-tui list'", query, len(matches))
	}

	return nil, -1, fmt.Errorf("профиль %q не найден", query)
}

func (s *Store) ActiveProfile() *Profile {
	if s.Settings.ActiveProfileID == "" {
		return nil
	}
	for i := range s.Profiles {
		if s.Profiles[i].ID == s.Settings.ActiveProfileID {
			return &s.Profiles[i]
		}
	}
	return nil
}

func (s *Store) FindSub(query string) (*Subscription, int, error) {
	query = strings.TrimSpace(query)
	if idx, err := strconv.Atoi(query); err == nil && idx >= 1 && idx <= len(s.Subscriptions) {
		return &s.Subscriptions[idx-1], idx - 1, nil
	}

	lower := strings.ToLower(query)
	for i := range s.Subscriptions {
		if s.Subscriptions[i].ID == query || strings.ToLower(s.Subscriptions[i].Name) == lower {
			return &s.Subscriptions[i], i, nil
		}
	}

	for i := range s.Subscriptions {
		if strings.Contains(strings.ToLower(s.Subscriptions[i].Name), lower) {
			return &s.Subscriptions[i], i, nil
		}
	}
	return nil, -1, fmt.Errorf("подписка %q не найдена", query)
}

func (s *Store) SubName(subID string) string {
	if subID == "" {
		return "локальный"
	}
	for _, sub := range s.Subscriptions {
		if sub.ID == subID {
			return sub.Name
		}
	}
	return subID
}

func (s *Store) ReplaceSubProfiles(subID string, incoming []Profile) {
	var oldActiveName string
	if p := s.ActiveProfile(); p != nil && p.SubID == subID {
		oldActiveName = p.Name
	}

	kept := make([]Profile, 0, len(s.Profiles)+len(incoming))
	for _, p := range s.Profiles {
		if p.SubID != subID {
			kept = append(kept, p)
		}
	}
	for i := range incoming {
		incoming[i].SubID = subID
		if oldActiveName != "" && incoming[i].Name == oldActiveName {
			s.Settings.ActiveProfileID = incoming[i].ID
		}
	}
	s.Profiles = append(kept, incoming...)
}

func (s *Store) RemoveProfile(idx int) {
	if idx < 0 || idx >= len(s.Profiles) {
		return
	}
	if s.Profiles[idx].ID == s.Settings.ActiveProfileID {
		s.Settings.ActiveProfileID = ""
	}
	s.Profiles = append(s.Profiles[:idx], s.Profiles[idx+1:]...)
}

func (s *Store) RemoveSub(idx int) int {
	if idx < 0 || idx >= len(s.Subscriptions) {
		return 0
	}
	subID := s.Subscriptions[idx].ID
	s.Subscriptions = append(s.Subscriptions[:idx], s.Subscriptions[idx+1:]...)

	kept := make([]Profile, 0, len(s.Profiles))
	removed := 0
	for _, p := range s.Profiles {
		if p.SubID == subID {
			if p.ID == s.Settings.ActiveProfileID {
				s.Settings.ActiveProfileID = ""
			}
			removed++
			continue
		}
		kept = append(kept, p)
	}
	s.Profiles = kept
	return removed
}

func NewID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func GenerateHWID() string {
	var seed string
	if raw, err := os.ReadFile("/etc/machine-id"); err == nil {
		seed = strings.TrimSpace(string(raw))
	}
	if seed == "" {
		seed, _ = os.Hostname()
	}
	sum := sha256.Sum256([]byte("xray-tui:" + seed))
	return hex.EncodeToString(sum[:16])
}

func RandomHWID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func DeviceOSVersion() string {
	if raw, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		return strings.TrimSpace(string(raw))
	}
	return "unknown"
}

func DeviceModel() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "linux-pc"
}