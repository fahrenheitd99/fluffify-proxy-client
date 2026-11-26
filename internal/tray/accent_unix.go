//go:build !windows

package tray

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// trayAccentPath — файл, за изменениями которого следит трей (KDE Plasma).
func trayAccentPath() string {
	return filepath.Join(userHomeDir(), ".config", "kdeglobals")
}

func trayAccent() string {
	return readKDEAccentColor(trayAccentPath())
}

// readKDEAccentColor извлекает системный акцентный цвет из ~/.config/kdeglobals.
func readKDEAccentColor(path string) string {
	const fallback = "#3daee9"
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}

	var generalAccent, selectionBg, viewActive string
	section := ""

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)

		switch {
		case section == "General" && k == "AccentColor":
			generalAccent = v
		case section == "Colors:Selection" && k == "BackgroundNormal":
			selectionBg = v
		case section == "Colors:View" && k == "ForegroundActive":
			viewActive = v
		}
	}

	for _, raw := range []string{generalAccent, selectionBg, viewActive} {
		if hex := parseKDEColorToHex(raw); hex != "" {
			return hex
		}
	}
	return fallback
}

func parseKDEColorToHex(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "#") {
		h := raw[1:]
		if len(h) == 6 {
			return "#" + h
		}
		if len(h) == 8 {
			return "#" + h[2:]
		}
		return ""
	}

	parts := strings.Split(raw, ",")
	if len(parts) < 3 {
		return ""
	}
	r, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	g, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	b, err3 := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err1 != nil || err2 != nil || err3 != nil {
		return ""
	}
	if r < 0 || r > 255 || g < 0 || g > 255 || b < 0 || b > 255 {
		return ""
	}
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}
