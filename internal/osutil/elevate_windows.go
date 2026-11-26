//go:build windows

package osutil

import (
	"errors"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var shellExecute = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW")

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

	// ShellExecuteW возвращает значение > 32 при успехе; 5 — доступ запрещён
	// (пользователь отклонил запрос UAC).
	ret, _, _ := shellExecute.Call(0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		0,
		uintptr(swShownormal))
	if ret <= 32 {
		if ret == 5 {
			return errors.New("запрос UAC отклонён: нужны права администратора")
		}
		return errors.New("не удалось перезапустить процесс от имени администратора")
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
