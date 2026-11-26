//go:build !windows

package osutil

import (
	"os"
	"path/filepath"
	"syscall"
)

func lockPath(name string) string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, name+".lock")
}

// TryLock захватывает межпроцессный flock. Блокировка снимается ядром
// при завершении процесса, поэтому «залипших» lock-файлов не остаётся.
func TryLock(name string) (unlock func(), ok bool) {
	f, err := os.OpenFile(lockPath(name), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, false
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, false
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, true
}
