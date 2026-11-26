//go:build !windows

package osutil

// EnsureCoreFiles — no-op: в Linux ядро ставится системным пакетом (xray)
// и ищется в PATH.
func EnsureCoreFiles() (string, error) {
	return "", nil
}

// CoreBinPath — пустая строка: runner найдёт xray в PATH.
func CoreBinPath() string {
	return ""
}
