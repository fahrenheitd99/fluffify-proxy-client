//go:build windows

package tui

import (
	"testing"
	"unsafe"
)

// Если эти проверки падают — структура разошлась с MIB_IF_ROW2 из iphlpapi.h.
func TestMibIfRow2Layout(t *testing.T) {
	if got, want := unsafe.Sizeof(mibIfRow2{}), uintptr(1360); got != want {
		t.Fatalf("sizeof(MIB_IF_ROW2) = %d, ждали %d", got, want)
	}
	if got, want := unsafe.Offsetof(mibIfRow2{}.Alias), uintptr(32); got != want {
		t.Errorf("offset Alias = %d, ждали %d", got, want)
	}
	if got, want := unsafe.Offsetof(mibIfRow2{}.InOctets), uintptr(1216); got != want {
		t.Errorf("offset InOctets = %d, ждали %d", got, want)
	}
	if got, want := unsafe.Offsetof(mibIfRow2{}.OutOctets), uintptr(1288); got != want {
		t.Errorf("offset OutOctets = %d, ждали %d", got, want)
	}
}
