//go:build windows

package osutil

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shellExecute       = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW")
	procCoInitializeEx = windows.NewLazySystemDLL("ole32.dll").NewProc("CoInitializeEx")
	procCoUninitialize = windows.NewLazySystemDLL("ole32.dll").NewProc("CoUninitialize")
)

const swShownormal = 1

// IsElevated сообщает, запущен ли процесс с правами администратора.
func IsElevated() bool {
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(&windows.SECURITY_NT_AUTHORITY, 2,
		windows.SECURITY_BUILTIN_DOMAIN_RID, windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0, &sid)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	member, err := windows.GetCurrentProcessToken().IsMember(sid)
	return err == nil && member
}

// RelaunchElevated запускает копию этого бинарника с аргументами от имени
// администратора: Windows покажет запрос UAC.
//
// ShellExecuteW требует инициализированный COM (STA) в вызывающем потоке —
// из фонового goroutine без COM окно UAC не показывается. Поэтому пиним
// поток, инициализируем COM и только потом зовём ShellExecuteW; если и это
// не сработало — принудительный UAC через PowerShell Start-Process -Verb RunAs.
func RelaunchElevated(args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	params, err := windows.UTF16PtrFromString(quoteArgs(args))
	if err != nil {
		return err
	}

	// ShellExecuteW работает надёжно только на потоке с COM (STA):
	// пиним поток, инициализируем COM, освобождаем после вызова.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	procCoInitializeEx.Call(0, windows.COINIT_APARTMENTTHREADED) // RPC_E_CHANGED_MODE игнорируем
	defer procCoUninitialize.Call()

	// ShellExecuteW возвращает значение > 32 при успехе; 5 — доступ запрещён
	// (пользователь отклонил запрос UAC).
	ret, _, _ := shellExecute.Call(0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		0,
		uintptr(swShownormal))
	if ret > 32 {
		return nil
	}
	if ret == 5 {
		return errors.New("запрос UAC отклонён: нужны права администратора")
	}

	// Принудительный UAC через PowerShell: он сам инициализирует COM и
	// надёжно показывает окно согласия даже из фоновых процессов.
	return relaunchViaPowerShell(exe, args)
}

// relaunchViaPowerShell — запасной способ элевации через PowerShell
// Start-Process -Verb RunAs. Не ждёт завершения дочернего процесса.
func relaunchViaPowerShell(exe string, args []string) error {
	psArgs := make([]string, 0, len(args))
	for _, a := range args {
		psArgs = append(psArgs, "'"+strings.ReplaceAll(a, "'", "''")+"'")
	}
	script := fmt.Sprintf(
		"Start-Process -FilePath '%s' -ArgumentList @(%s) -Verb RunAs -WindowStyle Hidden",
		strings.ReplaceAll(exe, "'", "''"), strings.Join(psArgs, ","))

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	HideChildConsole(cmd)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("не удалось показать запрос UAC (PowerShell): %w", err)
	}
	return nil
}

// quoteArgs экранирует аргументы командной строки для ShellExecuteW.
func quoteArgs(args []string) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\"") {
			parts = append(parts, `"`+strings.ReplaceAll(a, `"`, `\"`)+`"`)
			continue
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}
