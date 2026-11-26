package parser

import (
	"net/url"
	"strings"
)

// InputKind — что пользователь вставил в поле ввода.
type InputKind int

const (
	InputUnknown      InputKind = iota
	InputSubscription           // http:// или https:// — URL подписки
	InputLink                   // vless://, vmess://, hysteria2:// и т.п.
	InputJSON                   // сырой JSON: нода или правила
	InputFilePath               // путь к файлу
)

func (k InputKind) String() string {
	switch k {
	case InputSubscription:
		return "подписка"
	case InputLink:
		return "ключ"
	case InputJSON:
		return "JSON"
	case InputFilePath:
		return "файл"
	default:
		return "неизвестно"
	}
}

var knownSchemes = map[string]bool{
	"vless": true, "vmess": true, "trojan": true,
	"ss": true, "shadowsocks": true,
	"hy2": true, "hysteria2": true, "hysteria": true,
	"wireguard": true, "wg": true,
	"socks": true, "socks5": true,
}

// StripPasteMarkers убирает маркеры bracketed paste и управляющие символы,
// которые прилипают к вставленному тексту в консоли Windows.
func StripPasteMarkers(s string) string {
	s = strings.ReplaceAll(s, "\x1b[200~", "")
	s = strings.ReplaceAll(s, "\x1b[201~", "")
	s = strings.ReplaceAll(s, "\x1b[0~", "")
	s = strings.ReplaceAll(s, "[200~", "")
	s = strings.ReplaceAll(s, "[201~", "")
	return s
}

// NormalizeInput приводит вставленный текст к рабочему виду: снимает маркеры
// вставки, кавычки, а URL-кодированную строку декодирует (в том числе дважды).
func NormalizeInput(raw string) string {
	s := strings.TrimSpace(StripPasteMarkers(raw))
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') ||
			(s[0] == '`' && s[len(s)-1] == '`') {
			s = strings.TrimSpace(s[1 : len(s)-1])
		}
	}
	// Декодируем только то, что ещё не выглядит как готовая ссылка: у ссылки
	// уже есть схема, и её собственные %XX-последовательности трогать нельзя.
	for i := 0; i < 2 && strings.Contains(s, "%") && !strings.Contains(s, "://"); i++ {
		dec, err := url.QueryUnescape(s)
		if err != nil || dec == s {
			break
		}
		s = strings.TrimSpace(dec)
	}
	return s
}

// TotalLines возвращает непустые строки ввода — ключи часто вставляют пачкой.
func TotalLines(s string) []string {
	out := make([]string, 0, 4)
	for _, line := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' }) {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// ClassifyInput определяет тип ввода по содержимому.
//
//	А) http:// или https://  — подписка
//	Б) URL-кодированная строка — декодируется и классифицируется заново
//	В) vless://, vmess://, hysteria2://, trojan:// … — прямой ключ
//	Г) {…} или […] — сырой JSON
//	Д) всё остальное — путь к файлу
func ClassifyInput(raw string) (InputKind, string) {
	s := NormalizeInput(raw)
	if s == "" {
		return InputUnknown, ""
	}

	if strings.HasPrefix(s, "{") || strings.HasPrefix(s, "[") {
		return InputJSON, s
	}

	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return InputSubscription, s
	}

	if scheme, _, ok := strings.Cut(s, "://"); ok {
		if knownSchemes[strings.ToLower(strings.TrimSpace(scheme))] {
			return InputLink, s
		}
	}

	return InputFilePath, s
}
