//go:build !linux && !windows

package main

import "errors"

func runTUNSetup() error {
	return errors.New("настройка TUN поддерживается только в Linux и Windows")
}
