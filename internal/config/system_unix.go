//go:build !windows

package config

import (
	"os"
	"strings"
)

func machineSeed() string {
	if raw, err := os.ReadFile("/etc/machine-id"); err == nil {
		return strings.TrimSpace(string(raw))
	}
	return ""
}

func osVersion() string {
	if raw, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		return strings.TrimSpace(string(raw))
	}
	return "unknown"
}

func defaultDeviceModel() string {
	return "linux-pc"
}
