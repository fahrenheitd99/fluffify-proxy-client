package tray

import (
	"encoding/binary"
	"testing"
)

// Иконка ошибки должна быть строго ярко-красной (#FF0000), а не «розоватой».
func TestErrorIconColorIsPureRed(t *testing.T) {
	if errorIconColor != "#ff0000" {
		t.Fatalf("цвет ошибки = %q, ждали #ff0000", errorIconColor)
	}

	ico := tintICO(errorIconColor)
	le := binary.LittleEndian
	count := int(le.Uint16(ico[4:]))
	if count == 0 {
		t.Fatal("пустой ICO")
	}

	// Последняя запись — самая крупная (256×256).
	e := ico[6+(count-1)*16:]
	off := int(le.Uint32(e[12:]))
	width := int(le.Uint32(ico[off+4:]))
	pixels := off + 40

	for i := 0; i < width*width; i++ {
		p := pixels + i*4
		if ico[p+3] <= 200 {
			continue
		}
		r, g, b := ico[p+2], ico[p+1], ico[p+0] // DIB хранит BGRA
		if r != 0xff || g != 0 || b != 0 {
			t.Fatalf("пиксель иконки ошибки = #%02x%02x%02x, ждали #ff0000", r, g, b)
		}
		return
	}
	t.Fatal("в иконке ошибки нет непрозрачных пикселей")
}
