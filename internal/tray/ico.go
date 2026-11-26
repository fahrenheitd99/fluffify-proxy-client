package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"strings"

	"xray-tui/logo"
)

var icoSizes = []int{16, 20, 24, 32, 40, 48, 64, 128, 256}

// tintICO перекрашивает встроенную маску в заданный цвет и упаковывает её
// в многоразмерный .ico. systray на Windows грузит иконку через LoadImage,
// который принимает только ICO — PNG там не работает.
func tintICO(hex string) []byte {
	r, g, b, ok := parseHexColor(hex)
	if !ok {
		return logo.AppICO
	}
	mask := symbolicMask()
	if mask == nil {
		return logo.AppICO
	}
	return buildICO(r, g, b, mask, icoSizes)
}

// symbolicMask извлекает альфа-канал из встроенной PNG-маски.
func symbolicMask() *image.Alpha {
	src, err := png.Decode(bytes.NewReader(logo.SymbolicPNG))
	if err != nil {
		return nil
	}
	b := src.Bounds()
	m := image.NewAlpha(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			_, _, _, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			m.SetAlpha(x, y, color.Alpha{A: uint8(a >> 8)})
		}
	}
	return m
}

func parseHexColor(hex string) (uint8, uint8, uint8, bool) {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return uint8(v >> 16), uint8(v >> 8), uint8(v), true
}

func buildICO(r, g, b uint8, mask *image.Alpha, sizes []int) []byte {
	entries := make([][]byte, len(sizes))
	total := 6 + 16*len(sizes)
	for i, size := range sizes {
		entries[i] = dibEntry(size, r, g, b, mask)
		total += len(entries[i])
	}

	out := make([]byte, 0, total)
	out = append(out, 0, 0, 1, 0, byte(len(sizes)), byte(len(sizes)>>8))

	offset := 6 + 16*len(sizes)
	for i, size := range sizes {
		w, h := byte(size), byte(size)
		if size >= 256 {
			w, h = 0, 0
		}
		n := len(entries[i])
		out = append(out,
			w, h, 0, 0, // ширина, высота, палитра, резерв
			1, 0, 32, 0, // planes, bit count
			byte(n), byte(n>>8), byte(n>>16), byte(n>>24),
			byte(offset), byte(offset>>8), byte(offset>>16), byte(offset>>24),
		)
		offset += n
	}
	for _, entry := range entries {
		out = append(out, entry...)
	}
	return out
}

// dibEntry собирает одну запись ICO: BITMAPINFOHEADER + 32bpp BGRA (снизу вверх)
// + 1bpp AND-маска, которую читают старые оболочки.
func dibEntry(size int, r, g, b uint8, mask *image.Alpha) []byte {
	alpha := resizeAlpha(mask, size)
	maskRow := ((size + 31) / 32) * 4
	pixels := size * size * 4

	out := make([]byte, 40+pixels+maskRow*size)
	le := binary.LittleEndian
	le.PutUint32(out[0:], 40)
	le.PutUint32(out[4:], uint32(size))
	le.PutUint32(out[8:], uint32(size*2))
	le.PutUint16(out[12:], 1)
	le.PutUint16(out[14:], 32)
	le.PutUint32(out[20:], uint32(pixels+maskRow*size))

	p := 40
	for y := size - 1; y >= 0; y-- {
		for x := 0; x < size; x++ {
			out[p+0] = b
			out[p+1] = g
			out[p+2] = r
			out[p+3] = alpha.AlphaAt(x, y).A
			p += 4
		}
	}

	m := 40 + pixels
	for y := size - 1; y >= 0; y-- {
		row := out[m : m+maskRow]
		for x := 0; x < size; x++ {
			if alpha.AlphaAt(x, y).A < 128 {
				row[x/8] |= 1 << (7 - uint(x%8))
			}
		}
		m += maskRow
	}
	return out
}

// resizeAlpha уменьшает маску усреднением по площади — без внешних зависимостей.
func resizeAlpha(src *image.Alpha, size int) *image.Alpha {
	dst := image.NewAlpha(image.Rect(0, 0, size, size))
	sb := src.Bounds()
	for dy := 0; dy < size; dy++ {
		y0 := dy * sb.Dy() / size
		y1 := (dy + 1) * sb.Dy() / size
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for dx := 0; dx < size; dx++ {
			x0 := dx * sb.Dx() / size
			x1 := (dx + 1) * sb.Dx() / size
			if x1 <= x0 {
				x1 = x0 + 1
			}
			sum, n := 0, 0
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					sum += int(src.AlphaAt(x, y).A)
					n++
				}
			}
			dst.SetAlpha(dx, dy, color.Alpha{A: uint8(sum / n)})
		}
	}
	return dst
}
