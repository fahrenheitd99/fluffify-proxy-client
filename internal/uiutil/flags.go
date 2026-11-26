// Package uiutil готовит строки к показу в терминале: часть символов,
// которые понимает не всякий терминал, заменяется на текстовый эквивалент.
package uiutil

import "strings"

// regionalIndicatorBase — код 🇦; дальше идут 26 индикаторов, из которых
// складываются флаги стран (🇱🇹 = U+1F1F1 U+1F1F9).
const regionalIndicatorBase = 0x1F1E6

// Display приводит строку к виду, который одинаково рисуется везде: флаги
// стран заменяются тегами, 🇱🇹 → [LT].
//
// Windows Terminal (и старый conhost) такие эмодзи не рисует: вместо флага
// остаются буквы или кракозябры, и это ломает высоту строки в кадре. На Unix
// терминалы чаще всего умеют флаги, но вид интерфейса держим одинаковым.
func Display(s string) string {
	return replaceRegionalFlags(s)
}

// replaceRegionalFlags меняет пары региональных индикаторов на текстовые теги:
// 🇱🇹 → [LT].
func replaceRegionalFlags(s string) string {
	if !strings.ContainsFunc(s, isRegionalIndicator) {
		return s
	}

	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if !isRegionalIndicator(r) {
			b.WriteRune(r)
			continue
		}
		if i+1 < len(runes) && isRegionalIndicator(runes[i+1]) {
			b.WriteByte('[')
			b.WriteRune(regionalLetter(r))
			b.WriteRune(regionalLetter(runes[i+1]))
			b.WriteByte(']')
			i++
			continue
		}
		// Одинокий индикатор — просто выбрасываем.
	}
	return b.String()
}

func isRegionalIndicator(r rune) bool {
	return r >= regionalIndicatorBase && r <= regionalIndicatorBase+25
}

func regionalLetter(r rune) rune {
	return rune('A') + (r - regionalIndicatorBase)
}
