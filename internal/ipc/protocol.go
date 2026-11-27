package ipc

import (
	"encoding/json"
	"time"
)

type StartRequest struct {
	Profile   string          `json:"profile,omitempty"`
	Mode      string          `json:"mode,omitempty"`
	TUNName   string          `json:"tun_name,omitempty"`
	SocksAddr string          `json:"socks_addr,omitempty"`
	HTTPAddr  string          `json:"http_addr,omitempty"`
	Mixed     bool            `json:"mixed,omitempty"`
	ProxyAuth bool            `json:"proxy_auth,omitempty"`
	AssetDir  string          `json:"asset_dir,omitempty"`
	Config    json.RawMessage `json:"config,omitempty"`
}

type StatusResponse struct {
	Running   bool      `json:"running"`
	Profile   string    `json:"profile,omitempty"`
	Mode      string    `json:"mode,omitempty"`
	TUNName   string    `json:"tun_name,omitempty"`
	SocksAddr string    `json:"socks_addr,omitempty"`
	HTTPAddr  string    `json:"http_addr,omitempty"`
	Mixed     bool      `json:"mixed,omitempty"`
	ProxyAuth bool      `json:"proxy_auth,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	UptimeSec int64     `json:"uptime_sec"`
	// Elevated — запущен ли демон с правами администратора (нужно для TUN).
	Elevated bool `json:"elevated"`
}

type LogsResponse struct {
	Lines []string `json:"lines"`
}

// DevLogRequest — включение/выключение режима отладочных логов ядра.
type DevLogRequest struct {
	On bool `json:"on"`
}

// DevStatusResponse — сведения о запущенном ядре для dev-меню.
type DevStatusResponse struct {
	Running   bool   `json:"running"`
	PID       int    `json:"pid,omitempty"`
	Version   string `json:"version,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	UptimeSec int64  `json:"uptime_sec"`
	Profile   string `json:"profile,omitempty"`
	Mode      string `json:"mode,omitempty"`
	DevLog    bool   `json:"dev_log"`
}

// DevConfigResponse — дамп последнего конфига ядра (секреты маскируются).
type DevConfigResponse struct {
	Config string `json:"config"`
}

// DevCheckResponse — результат проверки туннеля (см. internal/netcheck).
type DevCheckResponse struct {
	TunnelIP    string   `json:"tunnel_ip"`
	DirectIP    string   `json:"direct_ip"`
	DirectIface string   `json:"direct_iface,omitempty"`
	ResolverIP  string   `json:"resolver_ip"`
	Notes       []string `json:"notes,omitempty"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

// DefaultSocketPath возвращает путь к сокету демона для текущей платформы.
func DefaultSocketPath() string {
	return defaultSocketPath()
}
