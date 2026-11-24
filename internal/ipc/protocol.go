package ipc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
}

type LogsResponse struct {
	Lines []string `json:"lines"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func DefaultSocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "xray-tui", "daemon.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("xray-tui-%d", os.Getuid()), "daemon.sock")
}