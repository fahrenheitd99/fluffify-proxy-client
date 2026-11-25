package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"

	"xray-tui/internal/config"
)

// terminalWidthRef — эталон для кадра: та же модель, что в termWidth
// (сумма runewidth.RuneWidth без селекторов начертания, не меньше uniseg).
func terminalWidthRef(s string) int {
	w := 0
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			i = skipEscape(s, i)
			continue
		}
		cl, _, _, _ := uniseg.FirstGraphemeClusterInString(s[i:], -1)
		sum := 0
		for _, r := range cl {
			if r == 0xFE0E || r == 0xFE0F {
				continue
			}
			sum += runewidth.RuneWidth(r)
		}
		if _, _, cw, _ := uniseg.FirstGraphemeClusterInString(cl, -1); cw > sum {
			sum = cw
		}
		w += sum
		i += len(cl)
	}
	return w
}

// Значения в столбце terminal получены замером позиции курсора (DSR \x1b[6n)
// в реальном терминале: курсор продвигается на сумму ширин нарисованных
// глифов. Столбец ours — то, что считает termWidth; он не должен быть меньше
// terminal, иначе строка завернётся и кадр «поплывёт».
func TestTermWidthVsTerminal(t *testing.T) {
	cases := []struct {
		glyph    string
		terminal int
		ours     int
	}{
		// ширина 1: обычный текст и боковые глифы
		{"США", 3, 3},
		{"─", 1, 1}, {"│", 1, 1}, {"┃", 1, 1}, {"▾", 1, 1},
		{"▸", 1, 1}, {"○", 1, 1}, {"●", 1, 1}, {"▄", 1, 1},
		// «текстовые» символы, которые терминал рисует в 1 клетку
		{"🛡", 1, 1}, {"🗺", 1, 1}, {"☠", 1, 1},
		{"✓", 1, 1}, {"✗", 1, 1}, {"✕", 1, 1}, {"☺", 1, 1},
		{"✂", 1, 1}, {"✈", 1, 1}, {"❤", 1, 1}, {"⚙", 1, 1}, {"⚠", 1, 1},
		{"⬇", 1, 1}, {"1️⃣", 1, 1},
		// ширина 2: эмодзи с Emoji_Presentation
		{"☕", 2, 2}, {"⭐", 2, 2}, {"⚡", 2, 2}, {"🎈", 2, 2},
		{"📺", 2, 2}, {"🎲", 2, 2}, {"🎮", 2, 2}, {"🔥", 2, 2},
		{"🧦", 2, 2}, {"🤖", 2, 2},
		// флаги: по одному региональному индикатору на клетку
		{"🇷🇺", 2, 2}, {"🇺🇸", 2, 2},
		// ZWJ: терминал рисует по частям
		{"🏴‍☠️", 3, 3}, {"👩‍💻", 4, 4}, {"👨‍👩‍👧", 6, 6},
		// селектор начертания на узкой базе: терминал рисует в 1 клетку,
		// мы сознательно считаем 2 — запас «никогда не занижать»
		{"⬇️", 1, 2}, {"❤️", 1, 2}, {"⚙️", 1, 2},
		{"⚠️", 1, 2}, {"☠️", 1, 2}, {"▶️", 1, 2},
	}
	for _, c := range cases {
		got := termWidth(c.glyph)
		if got != c.ours {
			t.Errorf("termWidth(%q) = %d, ждали %d", c.glyph, got, c.ours)
		}
		if got < c.terminal {
			t.Errorf("termWidth(%q) = %d ЗАНИЖАЕТ ширину терминала (%d)", c.glyph, got, c.terminal)
		}
	}
}

func emojiModel() model {
	store := &config.Store{}
	store.Settings.UIMode = config.UIModeKeys
	store.Settings.Mode = config.ModeProxy
	store.Settings.PingMode = config.PingModeProxyHEAD
	store.Subscriptions = []config.Subscription{{ID: "sub1", Name: "🧦 Носок VPN | Натяни безопасность"}}
	for i := 1; i <= 40; i++ {
		store.Profiles = append(store.Profiles, config.Profile{
			ID:       fmt.Sprintf("p%d", i),
			Name:     fmt.Sprintf("🇳🇱 Нидерланды ⭐️🏴‍☠️ %d", i),
			Protocol: "vless", SubID: "sub1",
		})
	}
	store.Settings.ActiveProfileID = store.Profiles[0].ID
	return model{store: store, collapsed: map[string]bool{}, pings: map[string]pingState{}, logCh: make(chan string, 1)}
}

// Каждая строка кадра должна занимать не больше клеток терминала, иначе
// терминал перенесёт её и кадр «поплывёт».
func TestFrameFitsTerminalWidth(t *testing.T) {
	applyTheme(config.ThemeDefaultDark, config.AccentCyan)
	for _, ui := range []string{config.UIModeKeys, config.UIModeButtons} {
		for _, tab := range []tabID{tabServers, tabSubs, tabLogs, tabSettings} {
			m := emojiModel()
			m.store.Settings.UIMode = ui
			m.tab = tab
			if tab == tabLogs {
				for i := 0; i < 300; i++ {
					m.logs = append(m.logs, fmt.Sprintf("[Info] %d 🏴‍☠️ Сервер 🧦 ⚡ node ok", i))
				}
				m.scrollLogsToBottom()
			}
			for _, sz := range [][2]int{{40, 20}, {48, 22}, {60, 24}, {100, 30}, {38, 12}} {
				m.width, m.height = sz[0], sz[1]
				for c := 0; c < 44; c++ {
					m.cursor = c
					m.subCursor = c % 3
					m.clampTreeScroll()
					ls := strings.Split(m.View(), "\n")
					if len(ls) != m.height {
						t.Fatalf("ui=%s tab=%d %dx%d: %d строк, ждали %d", ui, tab, m.width, m.height, len(ls), m.height)
					}
					for i, ln := range ls {
						if tw := terminalWidthRef(ln); tw > m.width {
							t.Errorf("ui=%s tab=%d %dx%d строка %d: ширина %d > %d", ui, tab, m.width, m.height, i, tw, m.width)
						}
					}
				}
			}
		}
	}
}

func TestWrappedLogLinesFitWidth(t *testing.T) {
	for _, w := range []int{40, 60, 100} {
		m := emojiModel()
		m.tab = tabLogs
		m.logs = []string{"🏴‍☠️ Сервер 🧦 ⚡ " + strings.Repeat("x", 120)}
		for i, ln := range m.wrappedLogLines(w) {
			if tw := terminalWidthRef(ln); tw > w-1 {
				t.Errorf("w=%d строка логов %d: ширина %d > %d", w, i, tw, w-1)
			}
		}
	}
}
