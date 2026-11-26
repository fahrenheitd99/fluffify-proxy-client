package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSanitizeInput(t *testing.T) {
	// \n и \r\n превращаются в пробелы, \t — тоже, NUL выбрасывается.
	if got := sanitizeInput("a\nb\r\nc\td\x00e"); got != "a b  c de" {
		t.Fatalf("sanitizeInput = %q, ждали %q", got, "a b  c de")
	}
}

func TestFieldValuePlaceholder(t *testing.T) {
	long := strings.Repeat("x", 500)
	got := fieldValue(long, true)
	if got != "[вставлено 500 символов]" {
		t.Fatalf("ожидали заглушку, получили %q", got)
	}
	if got := fieldValue(long, false); len(got) != 500 {
		t.Fatalf("без вставки текст должен показываться целиком, получили %d символов", len(got))
	}
	if got := fieldValue("", true); got != "" {
		t.Fatalf("пустое поле не должно превращаться в заглушку: %q", got)
	}
}

// Вставка в консоли Windows приходит пачкой KeyRunes — поле должно получить
// заглушку, но полный текст остаться в буфере.
func TestTrackInputBurstMarksPasted(t *testing.T) {
	m := model{subForm: subFormState{active: true, mode: subModeJSON}}

	for _, r := range "{\"outbounds\"" {
		m = m.trackInputBurst(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if !m.subForm.jsonPasted {
		t.Fatal("поток символов должен пометить поле как вставленное")
	}

	// Обычная печать после паузы возвращает показ текста.
	m.burst.at = time.Now().Add(-time.Second)
	m = m.trackInputBurst(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m.subForm.jsonPasted {
		t.Fatal("после паузы поле снова должно показывать текст")
	}
}

func TestTrackInputBurstBracketedPaste(t *testing.T) {
	m := model{subForm: subFormState{active: true, mode: subModeLink}}
	m = m.trackInputBurst(tea.KeyMsg{Type: tea.KeyRunes, Paste: true, Runes: []rune("vless://uuid@host:443")})
	if !m.subForm.linkPasted {
		t.Fatal("bracketed paste должен помечать поле как вставленное")
	}
}

func TestExtractInputTextSanitizes(t *testing.T) {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("line\r\nnext")}
	if got := extractInputText(msg); got != "line  next" {
		t.Fatalf("extractInputText = %q", got)
	}
}
