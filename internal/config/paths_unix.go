//go:build !windows

package config

import (
	"os"
	"path/filepath"
)

// ConfigDir — каталог настроек (XDG_CONFIG_HOME или ~/.config).
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

// GeoBaseDir — каталог для geoip.dat/geosite.dat (XDG_DATA_HOME или ~/.local/share).
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
