package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	// pasteBurstWindow — окно, в котором подряд идущие руны считаются вставкой.
	pasteBurstWindow = 60 * time.Millisecond
	// pasteBurstMin — сколько символов подряд превращают ввод во «вставку».
	pasteBurstMin = 3
)

// burstState считает символы, приходящие слитным потоком. В консоли Windows
// bracketed paste не поддерживается, и вставка приходит пачкой KeyRunes.
type burstState struct {
	at    time.Time
	count int
}

// sanitizeInput чистит текст, попадающий в текстовые поля: переводы строк и
// управляющие символы ломают вёрстку рамки.
func sanitizeInput(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f:
			return -1
		}
		return r
	}, s)
}

// pastePlaceholder заменяет длинный вставленный текст короткой заглушкой.
func pastePlaceholder(s string) string {
	return fmt.Sprintf("[вставлено %d символов]", len([]rune(s)))
}

// fieldValue решает, что показать в поле ввода. Полный текст всегда остаётся
// в самой переменной поля — на экран выводится либо он, либо заглушка.
func fieldValue(value string, pasted bool) string {
	if pasted && len([]rune(value)) > 0 {
		return pastePlaceholder(value)
	}
	return sanitizeInput(value)
}

// trackInputBurst вызывается до обработки клавиши: отмечает поля, в которые
// вставили текст, чтобы не рисовать длинную строку в рамке.
func (m model) trackInputBurst(msg tea.KeyMsg) model {
	if msg.Type != tea.KeyRunes || len(msg.Runes) == 0 {
		return m
	}

	now := time.Now()
	if msg.Paste || len(msg.Runes) > 1 || now.Sub(m.burst.at) < pasteBurstWindow {
		m.burst.count += len(msg.Runes)
	} else {
		m.burst.count = len(msg.Runes)
	}
	m.burst.at = now

	if msg.Paste || m.burst.count >= pasteBurstMin {
		m.markFieldPasted()
		return m
	}
	m.clearFieldPasted()
	return m
}

// markFieldPasted помечает активное текстовое поле как заполненное вставкой.
func (m *model) markFieldPasted() {
	switch {
	case m.subForm.active:
		switch m.subForm.mode {
		case subModeLink:
			m.subForm.linkPasted = true
		case subModeJSON:
			m.subForm.jsonPasted = true
		default:
			if m.subForm.step == 0 {
				m.subForm.urlPasted = true
			}
		}
	case m.jsonImportForm.active:
		if m.jsonImportForm.step == 1 {
			m.jsonImportForm.inputPasted = true
		}
	case m.settingsForm.kind != sFormNone:
		m.settingsForm.valuePasted = true
	}
}

// clearFieldPasted возвращает показ обычного текста: пользователь снова печатает.
func (m *model) clearFieldPasted() {
	m.subForm.linkPasted = false
	m.subForm.jsonPasted = false
	m.subForm.urlPasted = false
	m.jsonImportForm.inputPasted = false
	m.settingsForm.valuePasted = false
}
