package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

// Ширина строк в клетках терминала.
//
// Каждая строка списка, карточки и канваса добивается до полной ширины экрана,
// поэтому цена ошибки односторонняя: если ЗАНИЗИТЬ ширину хотя бы на клетку,
// строка станет шире терминала, терминал перенесёт её на вторую строку, кадр
// окажется выше экрана, altscreen начнёт скроллиться — и отрисовка «поплывёт»
// с наложением строк. Завышение безопасно: строка окажется на клетку короче.
//
// termWidth считает сумму runewidth.RuneWidth по рунам, потому что именно так
// терминал продвигает курсор:
//   - ZWJ-эмодзи рисуются по компонентам: 🏴‍☠️ = 2+1 = 3 клетки, 👨‍👩‍👧 = 6;
//     uniseg и runewidth.StringWidth схлопывают их к 2 и потому ЗАНИЖАЮТ;
//   - селекторы начертания (U+FE0E/U+FE0F) ширины не добавляют: ❤️ = 1, ⚙️ = 1;
//   - региональные индикаторы (флаги) считаются по одному: 🇷🇺 = 1+1 = 2;
//   - боковые глифы (─ │ ┃ ▾ ▸ ○ ●) — по 1 клетке.
//
// Чтобы никогда не занизить, сумма дополнительно не опускается ниже значения
// uniseg для этого графемного кластера.

// skipEscape возвращает индекс сразу за ANSI-последовательностью, начавшейся в i.
func skipEscape(s string, i int) int {
	j := i + 1
	if j >= len(s) {
		return j
	}
	switch s[j] {
	case '[': // CSI: ESC [ ... завершающий байт 0x40..0x7E
		j++
		for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
			j++
		}
		if j < len(s) {
			j++
		}
	case ']': // OSC: ESC ] ... BEL или ST
		j++
		for j < len(s) {
			if s[j] == 0x07 {
				return j + 1
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 2
			}
			j++
		}
	default:
		j++
	}
	return j
}

// clusterCells возвращает число клеток, которое терминал отводит под один
// графемный кластер: сумма runewidth.RuneWidth по рунам (без селекторов
// начертания), но не меньше ширины кластера по uniseg.
func clusterCells(cluster string) int {
	sum := 0
	for _, r := range cluster {
		if r == 0xFE0E || r == 0xFE0F {
			continue
		}
		sum += runewidth.RuneWidth(r)
	}
	if _, _, cw, _ := uniseg.FirstGraphemeClusterInString(cluster, -1); cw > sum {
		sum = cw
	}
	return sum
}

// termWidth измеряет строку в клетках терминала (ANSI-последовательности
// игнорируются).
func termWidth(s string) int {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	w := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i = skipEscape(s, i)
			continue
		}
		cluster, _, _, _ := uniseg.FirstGraphemeClusterInString(s[i:], -1)
		w += clusterCells(cluster)
		i += len(cluster)
	}
	return w
}

// truncateTerm обрезает строку до не более чем maxW клеток терминала,
// сохраняя ANSI-последовательности и не разрывая графемные кластеры.
func truncateTerm(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}

	var b strings.Builder
	used := 0
	cut := false
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			j := skipEscape(s, i)
			b.WriteString(s[i:j])
			i = j
			continue
		}
		cluster, _, _, _ := uniseg.FirstGraphemeClusterInString(s[i:], -1)
		c := clusterCells(cluster)
		if used+c > maxW {
			cut = true
			break
		}
		b.WriteString(cluster)
		used += c
		i += len(cluster)
	}
	if cut && strings.IndexByte(s, 0x1b) >= 0 {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}

type termCluster struct {
	text string
	w    int
}

// termClusters разбивает строку на графемные кластеры с их шириной в клетках.
func termClusters(s string) []termCluster {
	out := make([]termCluster, 0, len(s))
	for i := 0; i < len(s); {
		c, _, _, _ := uniseg.FirstGraphemeClusterInString(s[i:], -1)
		out = append(out, termCluster{text: c, w: clusterCells(c)})
		i += len(c)
	}
	return out
}

// lastBreak возвращает индекс сразу за последним пробелом/запятой (или 0).
func lastBreak(line string) int {
	if i := strings.LastIndexAny(line, " ,"); i >= 0 {
		return i + 1
	}
	return 0
}

// wrapTerm разбивает строку на строки длиной не более maxW клеток терминала,
// по возможности перенося по пробелу или запятой. Число строк логов считается
// именно по этой функции, поэтому ширину нужно мерить в клетках, а не в рунах:
// иначе строка с эмодзи занимает две строки терминала, а viewport и offset
// считают её как одну.
func wrapTerm(s string, maxW int) []string {
	if maxW <= 0 {
		return nil
	}
	var out []string
	line := ""
	lineW := 0
	for _, cl := range termClusters(s) {
		for lineW > 0 && lineW+cl.w > maxW {
			if idx := lastBreak(line); idx > 0 {
				out = append(out, line[:idx])
				line = strings.TrimLeft(line[idx:], " ")
				lineW = termWidth(line)
				continue
			}
			out = append(out, line)
			line = ""
			lineW = 0
		}
		line += cl.text
		lineW += cl.w
	}
	if line != "" {
		out = append(out, line)
	}
	if len(out) == 0 {
		out = append(out, "")
	}
	return out
}
