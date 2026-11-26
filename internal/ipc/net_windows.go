//go:build windows

package ipc

import (
	"context"
	"errors"
	"io/fs"
	"net"
	"syscall"
	"time"
)

// AF_UNIX на Windows поддерживается с Windows 10 1803.
const socketNetwork = "unix"

// Listen открывает слушающий сокет демона.
func Listen(path string) (net.Listener, error) {
	return net.Listen(socketNetwork, path)
}

// Dial подключается к сокету демона.
func Dial(ctx context.Context, path string) (net.Conn, error) {
	return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, socketNetwork, path)
}

// SecureSocket на Windows не нужен: AF_UNIX-сокет создаётся с правами каталога.
func SecureSocket(string) error {
	return nil
}

// IsDaemonDown отличает «демон не запущен» от прочих сетевых ошибок.
// Отсутствующий сокет приходит как ERROR_FILE_NOT_FOUND, а не как POSIX ENOENT.
func IsDaemonDown(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED)
}
