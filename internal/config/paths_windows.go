//go:build windows

package config

import (
	"os"
	"path/filepath"
)

// ConfigDir — каталог настроек в %AppData%\xray-tui.
func ConfigDir() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "xray-tui")
	}
	return ".xray-tui"
}

// GeoBaseDir — каталог для geoip.dat/geosite.dat в %LocalAppData%\xray-tui\geo.
// Roaming не используем: файлы крупные и не должны синхронизироваться профилем.
func GeoBaseDir() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "xray-tui", "geo")
	}
	return ".xray-tui-geo"
}
