//go:build windows

package autostart

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	valueName  = "Fluffify"
)

// Enabled сообщает, есть ли наш ярлык в HKCU\...\Run.
func Enabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(valueName)
	return err == nil && v != ""
}

// Set включает или выключает автозапуск трея при входе в систему.
func Set(enable bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	if !enable {
		_ = k.DeleteValue(valueName)
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return k.SetStringValue(valueName, `"`+exe+`" tray`)
}

// Start: на Windows демон поднимает сам трей, отдельной службы нет.
func Start() bool { return false }

// Stop: нечего останавливать — автозапуск срабатывает только при входе в систему.
func Stop() {}
