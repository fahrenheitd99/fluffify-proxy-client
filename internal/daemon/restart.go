package daemon

import (
	"context"
	"errors"
	"fmt"
	"time"

	"xray-tui/internal/ipc"
	"xray-tui/internal/osutil"
)

// StartElevated поднимает демон от имени администратора (Windows покажет запрос
// UAC) и ждёт, пока он начнёт отвечать. Используется при старте клиента, когда
// демон ещё не запущен: сам TUI при этом остаётся обычным процессом.
func StartElevated(ctx context.Context, socketPath string) error {
	if socketPath == "" {
		socketPath = ipc.DefaultSocketPath()
	}
	if !osutil.TunRequiresAdmin() {
		return errors.New("запуск демона с повышением прав нужен только в Windows")
	}
	// daemon-hidden — тот же демон, но с сразу скрытым окном консоли.
	if err := osutil.RelaunchElevated("daemon-hidden"); err != nil {
		return err
	}
	if !waitDaemonUp(ctx, ipc.NewClient(socketPath), 25*time.Second) {
		return errors.New("демон с правами администратора не запустился (запрос UAC отклонён?)")
	}
	return nil
}

// RestartElevated останавливает текущий демон и поднимает новый от имени
// администратора (Windows покажет запрос UAC). Нужно для TUN-режима, который
// требует прав на создание виртуального адаптера.
func RestartElevated(ctx context.Context, socketPath string) error {
	if socketPath == "" {
		socketPath = ipc.DefaultSocketPath()
	}
	if !osutil.TunRequiresAdmin() {
		return errors.New("перезапуск демона с повышением прав нужен только в Windows")
	}
	client := ipc.NewClient(socketPath)

	if _, err := client.Status(); err == nil {
		if err := client.Quit(); err != nil {
			return fmt.Errorf("не удалось остановить текущий демон: %w", err)
		}
		if !waitDaemonDown(client, 5*time.Second) {
			return errors.New("текущий демон не завершился")
		}
	}

	return StartElevated(ctx, socketPath)
}

func waitDaemonDown(client *ipc.Client, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := client.Status(); err != nil {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func waitDaemonUp(ctx context.Context, client *ipc.Client, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		if st, err := client.Status(); err == nil {
			return st.Elevated
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}
