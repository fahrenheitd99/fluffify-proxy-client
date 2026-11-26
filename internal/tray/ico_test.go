package tray

import (
	"encoding/binary"
	"testing"
)

func TestTintICOStructure(t *testing.T) {
	ico := tintICO("#ff3b30")
	if len(ico) < 6 {
		t.Fatalf("ICO слишком короткий: %d байт", len(ico))
	}
	le := binary.LittleEndian
	if got := le.Uint16(ico[0:]); got != 0 {
		t.Fatalf("reserved = %d, ждали 0", got)
	}
	if got := le.Uint16(ico[2:]); got != 1 {
		t.Fatalf("type = %d, ждали 1 (icon)", got)
	}
	count := int(le.Uint16(ico[4:]))
	if count != len(icoSizes) {
		t.Fatalf("записей %d, ждали %d", count, len(icoSizes))
	}

	dataStart := 6 + 16*count
	for i := 0; i < count; i++ {
		e := ico[6+i*16:]
		w := int(e[0])
		if w == 0 {
			w = 256
		}
		if w != icoSizes[i] {
			t.Errorf("запись %d: ширина %d, ждали %d", i, w, icoSizes[i])
		}
		size := int(le.Uint32(e[8:]))
		off := int(le.Uint32(e[12:]))
		if off < dataStart || off+size > len(ico) {
			t.Fatalf("запись %d: диапазон [%d,%d) вне файла (%d)", i, off, off+size, len(ico))
		}
		if got := le.Uint32(ico[off:]); got != 40 {
			t.Errorf("запись %d: biSize = %d, ждали 40", i, got)
		}
		if got := int(le.Uint32(ico[off+4:])); got != icoSizes[i] {
			t.Errorf("запись %d: biWidth = %d, ждали %d", i, got, icoSizes[i])
		}
		if got := int(le.Uint32(ico[off+8:])); got != icoSizes[i]*2 {
			t.Errorf("запись %d: biHeight = %d, ждали %d", i, got, icoSizes[i]*2)
		}
		if got := le.Uint16(ico[off+14:]); got != 32 {
			t.Errorf("запись %d: bitCount = %d, ждали 32", i, got)
		}
	}
}

func TestTintICOColorAndAlpha(t *testing.T) {
	ico := tintICO("#112233")
	le := binary.LittleEndian
	count := int(le.Uint16(ico[4:]))

	// Последняя запись — самая крупная (256×256).
	e := ico[6+(count-1)*16:]
	size := int(le.Uint32(e[8:]))
	off := int(le.Uint32(e[12:]))
	w := int(le.Uint32(ico[off+4:]))
	pixels := off + 40

	opaque, colored := 0, 0
	for i := 0; i < w*w; i++ {
		p := pixels + i*4
		if ico[p+3] <= 200 {
			continue
		}
		opaque++
		if ico[p+0] == 0x33 && ico[p+1] == 0x22 && ico[p+2] == 0x11 {
			colored++
		}
	}
	if opaque < 1000 {
		t.Fatalf("непрозрачных пикселей %d — маска пустая", opaque)
	}
	if colored != opaque {
		t.Fatalf("цвет залит не полностью: %d из %d", colored, opaque)
	}
	if size == 0 {
		t.Fatal("пустая запись ICO")
	}
}

func TestParseHexColor(t *testing.T) {
	if r, g, b, ok := parseHexColor(" #3DAEE9 "); !ok || r != 0x3d || g != 0xae || b != 0xe9 {
		t.Fatalf("parseHexColor = %x %x %x %v", r, g, b, ok)
	}
	for _, bad := range []string{"", "#12345", "#gggggg", "cyan"} {
		if _, _, _, ok := parseHexColor(bad); ok {
			t.Errorf("parseHexColor(%q) должен был вернуть ошибку", bad)
		}
	}
}
