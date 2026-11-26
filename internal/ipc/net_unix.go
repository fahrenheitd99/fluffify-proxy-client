//go:build !windows

package ipc

import (
	"context"
	"errors"
	"net"
	"os"
	"syscall"
	"time"
)

const socketNetwork = "unix"

// Listen открывает слушающий сокет демона.
func Listen(path string) (net.Listener, error) {
	return net.Listen(socketNetwork, path)
}

// Dial подключается к сокету демона.
func Dial(ctx context.Context, path string) (net.Conn, error) {
	return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, socketNetwork, path)
}

// SecureSocket ограничивает доступ к сокету владельцем.
func SecureSocket(path string) error {
	return os.Chmod(path, 0o600)
}

// IsDaemonDown отличает «демон не запущен» от прочих сетевых ошибок.
func IsDaemonDown(err error) bool {
	return errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED)
}
