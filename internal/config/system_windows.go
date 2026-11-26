//go:build windows

package config

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func machineSeed() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return ""
	}
	defer k.Close()
	if v, _, err := k.GetStringValue("MachineGuid"); err == nil {
		return strings.TrimSpace(v)
	}
	return ""
}

func osVersion() string {
	v := windows.RtlGetVersion()
	if v == nil {
		return "unknown"
	}
	return fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
}

func defaultDeviceModel() string {
	return "windows-pc"
}
