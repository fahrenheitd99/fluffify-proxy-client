package tray

import (
	"encoding/binary"
	"testing"

	"xray-tui/logo"
)

// Встроенные тематические иконки должны быть настоящими многоразмерными .ico:
// systray на Windows грузит их через LoadImage.
func TestEmbeddedThemeICOs(t *testing.T) {
	cases := map[string][]byte{
		"white": logo.SymbolicWhiteICO,
		"black": logo.SymbolicBlackICO,
	}
	for name, data := range cases {
		if len(data) < 6+16*len(icoSizes) {
			t.Fatalf("%s: файл слишком короткий (%d байт)", name, len(data))
		}
		if got := binary.LittleEndian.Uint16(data[0:]); got != 0 {
			t.Errorf("%s: reserved = %d", name, got)
		}
		if got := binary.LittleEndian.Uint16(data[2:]); got != 1 {
			t.Errorf("%s: type = %d, ждали 1", name, got)
		}
		count := int(binary.LittleEndian.Uint16(data[4:]))
		if count != len(icoSizes) {
			t.Errorf("%s: записей %d, ждали %d", name, count, len(icoSizes))
		}
		for i := 0; i < count; i++ {
			e := data[6+i*16:]
			size := int(binary.LittleEndian.Uint32(e[8:]))
			off := int(binary.LittleEndian.Uint32(e[12:]))
			if off < 6+16*count || off+size > len(data) {
				t.Fatalf("%s: запись %d вне файла", name, i)
			}
		}
	}
}
