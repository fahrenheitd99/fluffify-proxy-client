//go:build windows

package tray

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// Тёмное контекстное меню трея: недокументированный API uxtheme.dll,
// доступен с Windows 10 1903 (build 18362). До создания меню обновляем
// цветовую политику, включаем «разрешить тёмный» (AllowDark) и сбрасываем
// кэш тем — после этого popup-меню следует системной теме Windows
// (тёмной/светлой). Меню systray — стандартное Win32 (TrackPopupMenu),
// owner-draw не используется, поэтому хак применим целиком.
const (
	minMenuThemeBuild = 18362

	prefAppModeAllowDark = 1

	ordRefreshColorPolicy  = 104 // RefreshImmersiveColorPolicyState, необязателен
	ordSetPreferredAppMode = 135
	ordFlushMenuThemes     = 136
)

// applyMenuTheme включает следование меню системной теме. Идемпотентна:
// вызывается и на старте трей-процесса, и после инициализации окна systray.
// На старых системах или при ошибке загрузки молча ничего не делает —
// меню останется классическим (светлым).
func applyMenuTheme() {
	if _, _, build := windows.RtlGetNtVersionNumbers(); build < minMenuThemeBuild {
		return
	}

	h, err := windows.LoadLibrary("uxtheme.dll")
	if err != nil {
		return
	}
	// ВАЖНО: библиотеку не выгружаем. Если наша загрузка — единственная
	// ссылка на uxtheme в процессе, FreeLibrary выгрузит dll вместе с
	// выставленной темой; при следующей (ленивой) загрузке uxtheme
	// инициализируется заново со светлым дефолтом, и меню снова белое.
	_ = h

	setPreferredAppMode, err := windows.GetProcAddressByOrdinal(h, ordSetPreferredAppMode)
	if err != nil {
		return
	}
	flushMenuThemes, err := windows.GetProcAddressByOrdinal(h, ordFlushMenuThemes)
	if err != nil {
		return
	}

	// RefreshImmersiveColorPolicyState подтягивает цветовую политику из
	// реестра. Ординал необязателен: если его нет, пропускаем шаг и
	// продолжаем — не выходим.
	if refresh, err := windows.GetProcAddressByOrdinal(h, ordRefreshColorPolicy); err == nil {
		syscall.SyscallN(refresh)
	}

	syscall.SyscallN(setPreferredAppMode, prefAppModeAllowDark)
	syscall.SyscallN(flushMenuThemes)
}
