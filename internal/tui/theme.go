package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"xray-tui/internal/config"
)

const (
	statusButtonW = 7
	statusButtonH = 3
)

var (
	themeOrder = []string{
		config.ThemeSystem,
		config.ThemeDefaultDark,
		config.ThemeDefaultLight,
		config.ThemeGruvbox,
		config.ThemeOwo,
		config.ThemeUwu,
		config.ThemeGTAVI,
		config.ThemeHackerBlue,
		config.ThemeHackerGreen,
		config.ThemeHackerRed,
		config.ThemeAmberCRT,
		config.ThemePhosphorCRT,
		config.ThemeMatchaDark,
		config.ThemeMatchaLight,
		config.ThemeLavender,
		config.ThemePeach,
		config.ThemePaleLavender,
	}

	accentOrder = []string{
		config.AccentRed,
		config.AccentOrange,
		config.AccentYellow,
		config.AccentGreen,
		config.AccentCyan,
		config.AccentBlue,
		config.AccentViolet,
		config.AccentPink,
		config.AccentBrown,
		config.AccentWhite,
		config.AccentBlack,
	}

	colorAccent lipgloss.Color
	colorGreen  lipgloss.Color
	colorYellow lipgloss.Color
	colorRed    lipgloss.Color
	colorMuted  lipgloss.Color
	colorBorder lipgloss.Color

	styleHeaderBox  lipgloss.Style
	styleSectionBox lipgloss.Style
	styleTitle      lipgloss.Style
	styleMuted      lipgloss.Style
	styleGreen      lipgloss.Style
	styleYellow     lipgloss.Style
	styleRed        lipgloss.Style
	stylePingErr    lipgloss.Style
	styleAccent     lipgloss.Style
	styleTabActive  lipgloss.Style
	styleTabIdle    lipgloss.Style
	styleCursorRow  lipgloss.Style
	styleGroupRow   lipgloss.Style

	canvasEnabled  bool
	canvasBase     string
	canvasReplacer *strings.Replacer

	selReset    string
	selReplacer *strings.Replacer

	gradientEnabled bool
	gradBright      [][3]int
	gradDim         [][3]int

	themeIsLight bool
	accentRGB    [3]int
	redRGB       [3]int
	offFillRGB   [3]int
	offBorderRGB [3]int
	offTextRGB   [3]int
)

func init() {
	applyTheme(config.ThemeSystem, config.AccentCyan)
}

func themeLabel(theme string) string {
	switch theme {
	case config.ThemeDefaultDark:
		return "default Dark"
	case config.ThemeDefaultLight:
		return "default Light"
	case config.ThemeGruvbox:
		return "gruvbox"
	case config.ThemeOwo:
		return "owo"
	case config.ThemeUwu:
		return "uwu"
	case config.ThemeGTAVI:
		return "vice"
	case config.ThemeHackerBlue:
		return "hacker Blue"
	case config.ThemeHackerGreen:
		return "hacker Green"
	case config.ThemeHackerRed:
		return "hacker Red"
	case config.ThemeAmberCRT:
		return "amber CRT"
	case config.ThemePhosphorCRT:
		return "phosphor CRT"
	case config.ThemeMatchaDark:
		return "matcha Dark"
	case config.ThemeMatchaLight:
		return "matcha Light"
	case config.ThemeLavender:
		return "lavender"
	case config.ThemePeach:
		return "peach"
	default:
		return "system"
	case config.ThemePaleLavender:
		return "nhh"
	}
}

func accentLabel(accent string) string {
	switch accent {
	case config.AccentRed:
		return "Красный"
	case config.AccentOrange:
		return "Оранжевый"
	case config.AccentYellow:
		return "Жёлтый"
	case config.AccentGreen:
		return "Зелёный"
	case config.AccentBlue:
		return "Синий"
	case config.AccentViolet:
		return "Фиолетовый"
	case config.AccentPink:
		return "Розовый"
	case config.AccentBrown:
		return "Коричневый"
	case config.AccentWhite:
		return "Белый"
	case config.AccentBlack:
		return "Чёрный"
	default:
		return "Голубой"
	}
}

func resolveAccentColor(accent string, light bool) lipgloss.Color {
	if light {
		switch accent {
		case config.AccentRed:
			return lipgloss.Color("#d70000")
		case config.AccentOrange:
			return lipgloss.Color("#d75f00")
		case config.AccentYellow:
			return lipgloss.Color("#af8700")
		case config.AccentGreen:
			return lipgloss.Color("#008700")
		case config.AccentBlue:
			return lipgloss.Color("#005fd7")
		case config.AccentViolet:
			return lipgloss.Color("#8700af")
		case config.AccentPink:
			return lipgloss.Color("#d63384")
		case config.AccentBrown:
			return lipgloss.Color("#7f4f24")
		case config.AccentWhite:
			return lipgloss.Color("#ffffff")
		case config.AccentBlack:
			return lipgloss.Color("#141414")
		default:
			return lipgloss.Color("#0087af")
		}
	}

	switch accent {
	case config.AccentRed:
		return lipgloss.Color("#ff3b30")
	case config.AccentOrange:
		return lipgloss.Color("#ff9500")
	case config.AccentYellow:
		return lipgloss.Color("#ffd60a")
	case config.AccentGreen:
		return lipgloss.Color("#30d158")
	case config.AccentBlue:
		return lipgloss.Color("#0a84ff")
	case config.AccentViolet:
		return lipgloss.Color("#bf5af2")
	case config.AccentPink:
		return lipgloss.Color("#ff7eb6")
	case config.AccentBrown:
		return lipgloss.Color("#b07d56")
	case config.AccentWhite:
		return lipgloss.Color("#f4f4f6")
	case config.AccentBlack:
		return lipgloss.Color("#141418")
	default:
		return lipgloss.Color("#32d2f2")
	}
}

func hexToRGB(hex string) (int, int, int) {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return 0, 0, 0
	}
	r, _ := strconv.ParseInt(hex[0:2], 16, 64)
	g, _ := strconv.ParseInt(hex[2:4], 16, 64)
	b, _ := strconv.ParseInt(hex[4:6], 16, 64)
	return int(r), int(g), int(b)
}

func hexToArr(hex string) [3]int {
	r, g, b := hexToRGB(hex)
	return [3]int{r, g, b}
}

func rgbStops(hexes ...string) [][3]int {
	out := make([][3]int, len(hexes))
	for i, h := range hexes {
		out[i] = hexToArr(h)
	}
	return out
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func mixRGB(a, b [3]int, t float64) [3]int {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	return [3]int{
		clampInt(int(math.Round(float64(a[0])+(float64(b[0])-float64(a[0]))*t)), 0, 255),
		clampInt(int(math.Round(float64(a[1])+(float64(b[1])-float64(a[1]))*t)), 0, 255),
		clampInt(int(math.Round(float64(a[2])+(float64(b[2])-float64(a[2]))*t)), 0, 255),
	}
}

func dimDesaturateRGB(c [3]int, satFactor, valFactor float64) [3]int {
	gray := 0.299*float64(c[0]) + 0.587*float64(c[1]) + 0.114*float64(c[2])
	r := (gray + (float64(c[0])-gray)*satFactor) * valFactor
	g := (gray + (float64(c[1])-gray)*satFactor) * valFactor
	b := (gray + (float64(c[2])-gray)*satFactor) * valFactor
	return [3]int{
		clampInt(int(math.Round(r)), 0, 255),
		clampInt(int(math.Round(g)), 0, 255),
		clampInt(int(math.Round(b)), 0, 255),
	}
}

func contrastTextRGB(bg [3]int) [3]int {
	lum := 0.299*float64(bg[0]) + 0.587*float64(bg[1]) + 0.114*float64(bg[2])
	if lum > 165 {
		return [3]int{18, 18, 24}
	}
	return [3]int{255, 255, 255}
}

func fgRGB(c [3]int) string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c[0], c[1], c[2])
}

func bgRGB(c [3]int) string {
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", c[0], c[1], c[2])
}

func setCanvasRGB(fgHex, bgHex string) {
	fr, fg, fb := hexToRGB(fgHex)
	br, bg, bb := hexToRGB(bgHex)
	canvasEnabled = true
	fgSeq := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", fr, fg, fb)
	bgSeq := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", br, bg, bb)
	canvasBase = fmt.Sprintf("\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm", fr, fg, fb, br, bg, bb)
	canvasReplacer = strings.NewReplacer(
		"\x1b[0m", "\x1b[0m"+canvasBase,
		"\x1b[m", "\x1b[m"+canvasBase,
		"\x1b[39m", fgSeq,
		"\x1b[49m", bgSeq,
	)
}

func setSelectionRGB(fgHex, bgHex string) {
	br, bg, bb := hexToRGB(bgHex)
	bgSeq := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", br, bg, bb)
	if fgHex != "" {
		fr, fg, fb := hexToRGB(fgHex)
		fgSeq := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", fr, fg, fb)
		selReset = fmt.Sprintf("\x1b[0;38;2;%d;%d;%d;48;2;%d;%d;%dm", fr, fg, fb, br, bg, bb)
		selReplacer = strings.NewReplacer(
			"\x1b[0m", selReset,
			"\x1b[m", selReset,
			"\x1b[39m", fgSeq,
			"\x1b[49m", bgSeq,
		)
	} else {
		selReset = fmt.Sprintf("\x1b[0;48;2;%d;%d;%dm", br, bg, bb)
		selReplacer = strings.NewReplacer(
			"\x1b[0m", selReset,
			"\x1b[m", selReset,
			"\x1b[49m", bgSeq,
		)
	}
}

func applyTheme(theme, accent string) {
	gradientEnabled = false
	gradBright = nil
	gradDim = nil
	themeIsLight = false

	switch theme {
	case config.ThemeDefaultLight:
		themeIsLight = true
		setCanvasRGB("#262626", "#eeeeee")
		setSelectionRGB("#262626", "#d4d4d4")
		colorAccent = resolveAccentColor(accent, true)
		colorGreen = lipgloss.Color("#1f8b4c")
		colorYellow = lipgloss.Color("#b57c00")
		colorRed = lipgloss.Color("#d62828")
		colorMuted = lipgloss.Color("#707070")
		colorBorder = lipgloss.Color("#b8b8b8")
		offFillRGB = [3]int{214, 214, 220}
		offBorderRGB = [3]int{168, 168, 178}
		offTextRGB = [3]int{70, 70, 80}

	case config.ThemeDefaultDark:
		setCanvasRGB("#dadada", "#1c1c1c")
		setSelectionRGB("#dadada", "#333333")
		colorAccent = resolveAccentColor(accent, false)
		colorGreen = lipgloss.Color("#30d158")
		colorYellow = lipgloss.Color("#ffd60a")
		colorRed = lipgloss.Color("#ff453a")
		colorMuted = lipgloss.Color("#8a8a8a")
		colorBorder = lipgloss.Color("#484848")
		offFillRGB = [3]int{44, 44, 50}
		offBorderRGB = [3]int{82, 82, 92}
		offTextRGB = [3]int{196, 196, 208}

	case config.ThemeGruvbox:
		setCanvasRGB("#ebdbb2", "#282828")
		setSelectionRGB("#ebdbb2", "#3c3836")
		colorAccent = lipgloss.Color("#fe8019")
		colorGreen = lipgloss.Color("#b8bb26")
		colorYellow = lipgloss.Color("#fabd2f")
		colorRed = lipgloss.Color("#fb4934")
		colorMuted = lipgloss.Color("#a89984")
		colorBorder = lipgloss.Color("#504945")
		offFillRGB = [3]int{60, 56, 54}
		offBorderRGB = [3]int{102, 92, 84}
		offTextRGB = [3]int{213, 196, 161}

	case config.ThemeOwo:
		themeIsLight = true
		setCanvasRGB("#593748", "#fff0f6")
		setSelectionRGB("#593748", "#f8d7e6")
		colorAccent = lipgloss.Color("#d94f8a")
		colorGreen = lipgloss.Color("#3b9a6d")
		colorYellow = lipgloss.Color("#c77d1a")
		colorRed = lipgloss.Color("#d63a5c")
		colorMuted = lipgloss.Color("#a67c92")
		colorBorder = lipgloss.Color("#e8b4cc")
		offFillRGB = [3]int{245, 214, 228}
		offBorderRGB = [3]int{214, 162, 188}
		offTextRGB = [3]int{110, 68, 90}

	case config.ThemeUwu:
		setCanvasRGB("#f4dce8", "#1b141a")
		setSelectionRGB("#f4dce8", "#2d202b")
		colorAccent = lipgloss.Color("#f497c2")
		colorGreen = lipgloss.Color("#9be3b8")
		colorYellow = lipgloss.Color("#f7d08a")
		colorRed = lipgloss.Color("#f28b9c")
		colorMuted = lipgloss.Color("#9d7d8f")
		colorBorder = lipgloss.Color("#4a3545")
		offFillRGB = [3]int{45, 32, 43}
		offBorderRGB = [3]int{92, 66, 86}
		offTextRGB = [3]int{212, 178, 198}

	case config.ThemeGTAVI:
		setCanvasRGB("#ecebf0", "#0c0b0e")
		setSelectionRGB("#ecebf0", "#1e1c24")
		colorAccent = lipgloss.Color("#ff52a8")
		colorGreen = lipgloss.Color("#4ef0a3")
		colorYellow = lipgloss.Color("#ffb85c")
		colorRed = lipgloss.Color("#ff4569")
		colorMuted = lipgloss.Color("#767280")
		colorBorder = lipgloss.Color("#2e2a36")
		offFillRGB = [3]int{32, 29, 40}
		offBorderRGB = [3]int{74, 68, 88}
		offTextRGB = [3]int{188, 184, 200}

		gradientEnabled = true
		gradBright = rgbStops("#5478ff", "#ff2ea6", "#ffb84d")
		gradDim = rgbStops("#3a4270", "#6e2956", "#734e38")

	case config.ThemeHackerBlue:
		setCanvasRGB("#c8e6ff", "#070c14")
		setSelectionRGB("#e2f2ff", "#11223a")
		colorAccent = lipgloss.Color("#00b4ff")
		colorGreen = lipgloss.Color("#22f3c3")
		colorYellow = lipgloss.Color("#ffcf56")
		colorRed = lipgloss.Color("#ff4d6d")
		colorMuted = lipgloss.Color("#4a6b8a")
		colorBorder = lipgloss.Color("#183454")
		offFillRGB = [3]int{15, 29, 48}
		offBorderRGB = [3]int{28, 62, 98}
		offTextRGB = [3]int{120, 175, 220}

	case config.ThemeHackerGreen:
		setCanvasRGB("#b8ffca", "#050d07")
		setSelectionRGB("#d8ffe2", "#0f2916")
		colorAccent = lipgloss.Color("#00ff66")
		colorGreen = lipgloss.Color("#39ff14")
		colorYellow = lipgloss.Color("#d4ff33")
		colorRed = lipgloss.Color("#ff3b5c")
		colorMuted = lipgloss.Color("#3c734c")
		colorBorder = lipgloss.Color("#164223")
		offFillRGB = [3]int{13, 33, 19}
		offBorderRGB = [3]int{26, 74, 40}
		offTextRGB = [3]int{110, 200, 138}

	case config.ThemeHackerRed:
		setCanvasRGB("#ffd6dc", "#0d0608")
		setSelectionRGB("#ffe8ec", "#2b1016")
		colorAccent = lipgloss.Color("#ff2a4b")
		colorGreen = lipgloss.Color("#4ef09c")
		colorYellow = lipgloss.Color("#ff9e3d")
		colorRed = lipgloss.Color("#ff1744")
		colorMuted = lipgloss.Color("#7d444f")
		colorBorder = lipgloss.Color("#4a1822")
		offFillRGB = [3]int{36, 15, 20}
		offBorderRGB = [3]int{86, 30, 42}
		offTextRGB = [3]int{214, 134, 148}

	case config.ThemeAmberCRT:
		setCanvasRGB("#ffd27d", "#0e0a04")
		setSelectionRGB("#ffe6b3", "#2b1e0a")
		colorAccent = lipgloss.Color("#ffb000")
		colorGreen = lipgloss.Color("#b8d943")
		colorYellow = lipgloss.Color("#ffd447")
		colorRed = lipgloss.Color("#ff5533")
		colorMuted = lipgloss.Color("#85632a")
		colorBorder = lipgloss.Color("#4d3612")
		offFillRGB = [3]int{36, 25, 9}
		offBorderRGB = [3]int{88, 60, 18}
		offTextRGB = [3]int{212, 162, 74}

	case config.ThemePhosphorCRT:
		setCanvasRGB("#4df3ff", "#000000")
		setSelectionRGB("#000000", "#f5ff00")
		colorAccent = lipgloss.Color("#ff00cc")
		colorGreen = lipgloss.Color("#00ff66")
		colorYellow = lipgloss.Color("#f5ff00")
		colorRed = lipgloss.Color("#ff1744")
		colorMuted = lipgloss.Color("#d94848")
		colorBorder = lipgloss.Color("#ff2a2a")
		offFillRGB = [3]int{48, 8, 14}
		offBorderRGB = [3]int{255, 42, 42}
		offTextRGB = [3]int{255, 196, 196}

	case config.ThemeMatchaDark:
		setCanvasRGB("#dce6d5", "#171c16")
		setSelectionRGB("#eaf2e4", "#263024")
		colorAccent = lipgloss.Color("#95c98a")
		colorGreen = lipgloss.Color("#8fd99a")
		colorYellow = lipgloss.Color("#e8cf8e")
		colorRed = lipgloss.Color("#e38b8b")
		colorMuted = lipgloss.Color("#7c8f78")
		colorBorder = lipgloss.Color("#354233")
		offFillRGB = [3]int{36, 44, 34}
		offBorderRGB = [3]int{70, 86, 66}
		offTextRGB = [3]int{184, 200, 178}

	case config.ThemeMatchaLight:
		themeIsLight = true
		setCanvasRGB("#2f3b2c", "#f3f7ee")
		setSelectionRGB("#2f3b2c", "#dce8d2")
		colorAccent = lipgloss.Color("#4f8a4b")
		colorGreen = lipgloss.Color("#3b8252")
		colorYellow = lipgloss.Color("#b5892a")
		colorRed = lipgloss.Color("#c95454")
		colorMuted = lipgloss.Color("#778a72")
		colorBorder = lipgloss.Color("#c2d4b8")
		offFillRGB = [3]int{218, 230, 210}
		offBorderRGB = [3]int{172, 194, 160}
		offTextRGB = [3]int{74, 94, 70}

	case config.ThemeLavender:
		setCanvasRGB("#e6dff5", "#16141f")
		setSelectionRGB("#f0ebfc", "#272338")
		colorAccent = lipgloss.Color("#b69df8")
		colorGreen = lipgloss.Color("#94e2b8")
		colorYellow = lipgloss.Color("#f5d796")
		colorRed = lipgloss.Color("#f28fa3")
		colorMuted = lipgloss.Color("#837a9e")
		colorBorder = lipgloss.Color("#3b3454")
		offFillRGB = [3]int{37, 33, 52}
		offBorderRGB = [3]int{74, 66, 102}
		offTextRGB = [3]int{196, 186, 222}

	case config.ThemePeach:
		themeIsLight = true
		setCanvasRGB("#4d3328", "#fff3eb")
		setSelectionRGB("#4d3328", "#fae0cf")
		colorAccent = lipgloss.Color("#e0724a")
		colorGreen = lipgloss.Color("#439463")
		colorYellow = lipgloss.Color("#c4821f")
		colorRed = lipgloss.Color("#d44d5c")
		colorMuted = lipgloss.Color("#9e7a6b")
		colorBorder = lipgloss.Color("#ebc6b2")
		offFillRGB = [3]int{247, 222, 206}
		offBorderRGB = [3]int{219, 175, 150}
		offTextRGB = [3]int{110, 74, 58}

	case config.ThemePaleLavender:
		setCanvasRGB("#dce7f2", "#112233")
		setSelectionRGB("#eef5fc", "#21405e")
		colorAccent = lipgloss.Color("#1de954")
		colorGreen = lipgloss.Color("#38f276")
		colorYellow = lipgloss.Color("#e8d8a0")
		colorRed = lipgloss.Color("#f26d7d")
		colorMuted = lipgloss.Color("#7395b8")
		colorBorder = lipgloss.Color("#31567a")
		offFillRGB = [3]int{26, 49, 74}
		offBorderRGB = [3]int{58, 96, 134}
		offTextRGB = [3]int{184, 208, 232}

	default: // System — прозрачный фон терминала
		canvasEnabled = false
		canvasBase = ""
		canvasReplacer = nil
		setSelectionRGB("", "#3a3a3a")
		colorAccent = resolveAccentColor(accent, false)
		colorGreen = lipgloss.Color("#30d158")
		colorYellow = lipgloss.Color("#ffd60a")
		colorRed = lipgloss.Color("#ff453a")
		colorMuted = lipgloss.Color("#888888")
		colorBorder = lipgloss.Color("#4e4e4e")
		offFillRGB = [3]int{44, 44, 50}
		offBorderRGB = [3]int{86, 86, 96}
		offTextRGB = [3]int{198, 198, 210}
	}

	accentRGB = hexToArr(string(colorAccent))
	redRGB = hexToArr(string(colorRed))

	styleHeaderBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1)

	styleSectionBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1)

	styleTitle = lipgloss.NewStyle().Bold(true)
	styleMuted = lipgloss.NewStyle().Foreground(colorMuted)
	styleGreen = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	styleYellow = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
	styleRed = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
	stylePingErr = lipgloss.NewStyle().Foreground(lipgloss.Color("#111111")).Background(colorRed).Bold(true)
	styleAccent = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleTabActive = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Underline(true)
	styleTabIdle = lipgloss.NewStyle().Foreground(colorMuted)
	styleCursorRow = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleGroupRow = lipgloss.NewStyle().Bold(true)
}

// --- Градиенты и кнопка статуса ---

func evalGradient(stops [][3]int, t float64) [3]int {
	if len(stops) == 0 {
		return [3]int{255, 255, 255}
	}
	if len(stops) == 1 || t <= 0 {
		return stops[0]
	}
	if t >= 1 {
		return stops[len(stops)-1]
	}
	scaled := t * float64(len(stops)-1)
	idx := int(math.Floor(scaled))
	if idx >= len(stops)-1 {
		return stops[len(stops)-1]
	}
	f := scaled - float64(idx)
	return mixRGB(stops[idx], stops[idx+1], f)
}

func isEmojiRune(r rune) bool {
	return r >= 0x1F000 || r == 0xFE0F || r == 0x200D || (r >= 0x2600 && r <= 0x27BF)
}

func renderGradientText(text string, bold, underline bool, stops [][3]int) string {
	runes := []rune(text)
	textCount := 0
	for _, r := range runes {
		if !isEmojiRune(r) {
			textCount++
		}
	}

	var b strings.Builder
	if bold {
		b.WriteString("\x1b[1m")
	}
	if underline {
		b.WriteString("\x1b[4m")
	}

	pos := 0
	for _, r := range runes {
		if isEmojiRune(r) {
			b.WriteString("\x1b[39m")
			b.WriteRune(r)
			continue
		}
		t := 0.0
		if textCount > 1 {
			t = float64(pos) / float64(textCount-1)
		}
		c := evalGradient(stops, t)
		fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm%c", c[0], c[1], c[2], r)
		pos++
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

func renderTabActive(label string) string {
	if gradientEnabled {
		return renderGradientText(label, true, true, gradBright)
	}
	return styleTabActive.Render(label)
}

func renderSeparator(w int) string {
	if !gradientEnabled || w <= 0 {
		return styleMuted.Render(strings.Repeat("─", w))
	}
	var b strings.Builder
	for x := 0; x < w; x++ {
		t := 0.0
		if w > 1 {
			t = float64(x) / float64(w-1)
		}
		c := evalGradient(gradBright, t)
		fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm─", c[0], c[1], c[2])
	}
	b.WriteString("\x1b[0m")
	return b.String()
}

func renderGradientBox(rows []string, totalW int, stops [][3]int) string {
	innerW := totalW - 4
	if innerW < 10 {
		innerW = 10
		totalW = innerW + 4
	}
	totalH := len(rows) + 2

	colorAt := func(x, y int) string {
		tx := 0.0
		if totalW > 1 {
			tx = float64(x) / float64(totalW-1)
		}
		ty := 0.0
		if totalH > 1 {
			ty = float64(y) / float64(totalH-1)
		}
		t := 0.70*ty + 0.30*tx
		return fgRGB(evalGradient(stops, t))
	}

	out := make([]string, 0, totalH)

	var top strings.Builder
	top.WriteString(colorAt(0, 0) + "╭")
	for x := 1; x < totalW-1; x++ {
		top.WriteString(colorAt(x, 0) + "─")
	}
	top.WriteString(colorAt(totalW-1, 0) + "╮\x1b[0m")
	out = append(out, top.String())

	for i, row := range rows {
		y := i + 1
		clipped := truncateTerm(row, innerW)
		pad := innerW - termWidth(clipped)
		if pad < 0 {
			pad = 0
		}
		leftBar := colorAt(0, y) + "│\x1b[0m"
		rightBar := colorAt(totalW-1, y) + "│\x1b[0m"
		out = append(out, leftBar+" "+clipped+strings.Repeat(" ", pad)+" "+rightBar)
	}

	var bot strings.Builder
	yBot := totalH - 1
	bot.WriteString(colorAt(0, yBot) + "╰")
	for x := 1; x < totalW-1; x++ {
		bot.WriteString(colorAt(x, yBot) + "─")
	}
	bot.WriteString(colorAt(totalW-1, yBot) + "╯\x1b[0m")
	out = append(out, bot.String())

	return strings.Join(out, "\n")
}

// renderPlainBox рисует карточку с рамкой вручную, добивая каждую строку до
// нужной ширины по termWidth. Штатный lipgloss-бокс (.Width) добивает строку
// до innerW по своей таблице ширины, из-за чего строка с эмодзи выходит за
// правый край экрана и терминал переносит её на вторую строку.
func renderPlainBox(rows []string, totalW int, border lipgloss.TerminalColor) string {
	innerW := totalW - 4
	if innerW < 10 {
		innerW = 10
		totalW = innerW + 4
	}
	b := lipgloss.NewStyle().Foreground(border)

	out := make([]string, 0, len(rows)+2)
	out = append(out, b.Render("╭"+strings.Repeat("─", totalW-2)+"╮"))
	for _, row := range rows {
		content := truncateTerm(row, innerW)
		pad := innerW - termWidth(content)
		if pad < 0 {
			pad = 0
		}
		out = append(out, b.Render("│")+" "+content+strings.Repeat(" ", pad)+" "+b.Render("│"))
	}
	out = append(out, b.Render("╰"+strings.Repeat("─", totalW-2)+"╯"))
	return strings.Join(out, "\n")
}

func renderHeaderCard(lines []string, totalW int, state string) string {
	if gradientEnabled && state != "err" {
		stops := gradDim
		if state == "on" {
			stops = gradBright
		}
		return renderGradientBox(lines, totalW, stops)
	}

	border := colorBorder
	switch state {
	case "on":
		border = colorAccent
	case "err":
		border = colorRed
	}
	return renderPlainBox(lines, totalW, border)
}

func renderSectionCard(title string, rows []string, totalW int) string {
	if gradientEnabled {
		header := renderGradientText(title, true, false, gradBright)
		all := append([]string{header}, rows...)
		return renderGradientBox(all, totalW, gradDim)
	}
	content := append([]string{styleTitle.Render(title)}, rows...)
	return renderPlainBox(content, totalW, colorBorder)
}

func brailleWaveRunes(width, tick int, phaseOffset float64) []rune {
	leftDots := [4]rune{0x01, 0x02, 0x04, 0x40}
	rightDots := [4]rune{0x08, 0x10, 0x20, 0x80}
	out := make([]rune, width)
	for col := 0; col < width; col++ {
		x0 := float64(col*2)*0.68 - float64(tick)*0.55 + phaseOffset
		x1 := float64(col*2+1)*0.68 - float64(tick)*0.55 + phaseOffset
		l0 := clampInt(int(math.Round((math.Sin(x0)+1.0)*1.5)), 0, 3)
		l1 := clampInt(int(math.Round((math.Sin(x1)+1.0)*1.5)), 0, 3)
		out[col] = 0x2800 | leftDots[l0] | rightDots[l1]
	}
	return out
}

// renderStatusButton рисует скруглённый квадрат 7×3 без внешней рамки
// с акцентной (или градиентной в Vice) заливкой, синусоидой и анимациями входа/выхода цвета.
func renderStatusButton(state string, waving bool, animTick int, revealStep int, hideStep int) [statusButtonH]string {
	activeFillAt := func(col, row int) [3]int {
		if gradientEnabled {
			t := 0.72*(float64(col)/6.0) + 0.28*(float64(row)/2.0)
			return evalGradient(gradBright, t)
		}
		return accentRGB
	}

	var rowFill [3]float64
	var midText string
	var textAlpha float64
	isErr := state == "err" && !waving && revealStep == 0 && hideStep == 0
	showWave := waving && revealStep == 0 && hideStep == 0

	switch {
	case showWave:
		rowFill = [3]float64{0, 0, 0}
		midText = "  OFF  "
		textAlpha = 1.0
	case revealStep > 0:
		switch revealStep {
		case 1:
			rowFill = [3]float64{0, 0, 1.0}
			midText = "  OFF  "
			textAlpha = 0.45
		case 2:
			rowFill = [3]float64{0, 0.55, 1.0}
			midText = "       "
		case 3:
			rowFill = [3]float64{0.55, 1.0, 1.0}
			midText = "       "
		case 4:
			rowFill = [3]float64{1.0, 1.0, 1.0}
			midText = "       "
		case 5:
			rowFill = [3]float64{1.0, 1.0, 1.0}
			midText = "  O    "
			textAlpha = 0.45
		case 6:
			rowFill = [3]float64{1.0, 1.0, 1.0}
			midText = "  O N  "
			textAlpha = 0.65
		case 7:
			rowFill = [3]float64{1.0, 1.0, 1.0}
			midText = "  O N  "
			textAlpha = 0.85
		default:
			rowFill = [3]float64{1.0, 1.0, 1.0}
			midText = "  O N  "
			textAlpha = 1.0
		}
	case hideStep > 0:
		// Цвет уходит вверх (очищается снизу вверх) с той же скоростью, затем проявляется OFF
		switch hideStep {
		case 1:
			rowFill = [3]float64{1.0, 1.0, 0.0}
			midText = "  O N  "
			textAlpha = 0.45
		case 2:
			rowFill = [3]float64{1.0, 0.45, 0.0}
			midText = "       "
		case 3:
			rowFill = [3]float64{0.45, 0.0, 0.0}
			midText = "       "
		case 4:
			rowFill = [3]float64{0.0, 0.0, 0.0}
			midText = "       "
		case 5:
			rowFill = [3]float64{0.0, 0.0, 0.0}
			midText = "  O    "
			textAlpha = 0.45
		case 6:
			rowFill = [3]float64{0.0, 0.0, 0.0}
			midText = "  OF   "
			textAlpha = 0.65
		case 7:
			rowFill = [3]float64{0.0, 0.0, 0.0}
			midText = "  OFF  "
			textAlpha = 0.85
		default:
			rowFill = [3]float64{0.0, 0.0, 0.0}
			midText = "  OFF  "
			textAlpha = 1.0
		}
	case isErr:
		rowFill = [3]float64{1, 1, 1}
		midText = "  ERR  "
		textAlpha = 1.0
	case state == "on":
		rowFill = [3]float64{1, 1, 1}
		midText = "  O N  "
		textAlpha = 1.0
	default:
		rowFill = [3]float64{0, 0, 0}
		midText = "  OFF  "
		textAlpha = 1.0
	}

	cellFill := func(col, innerRow int) [3]int {
		if isErr {
			return redRGB
		}
		f := rowFill[innerRow]
		if f <= 0 {
			return offFillRGB
		}
		target := activeFillAt(col, innerRow)
		if f >= 1 {
			return target
		}
		return mixRGB(offFillRGB, target, f)
	}

	renderCapRow := func(innerRow int, edgeGlyph rune, wavePhase float64) string {
		var b strings.Builder
		b.WriteString(fgRGB(cellFill(0, innerRow)))
		b.WriteRune(edgeGlyph)
		b.WriteString("\x1b[0m")

		if showWave {
			waves := brailleWaveRunes(5, animTick, wavePhase)
			for i := 0; i < 5; i++ {
				col := i + 1
				b.WriteString(bgRGB(offFillRGB) + fgRGB(activeFillAt(col, innerRow)))
				b.WriteRune(waves[i])
			}
		} else {
			for col := 1; col <= 5; col++ {
				b.WriteString(bgRGB(cellFill(col, innerRow)) + " ")
			}
		}
		b.WriteString("\x1b[0m")

		b.WriteString(fgRGB(cellFill(6, innerRow)))
		b.WriteRune(edgeGlyph)
		b.WriteString("\x1b[0m")
		return b.String()
	}

	renderMidRow := func() string {
		var b strings.Builder
		chars := []rune(midText)
		for col := 0; col < 7; col++ {
			ch := ' '
			if col < len(chars) {
				ch = chars[col]
			}
			bg := cellFill(col, 1)
			var targetFg [3]int
			if rowFill[1] < 0.5 && !isErr {
				targetFg = offTextRGB
			} else {
				targetFg = contrastTextRGB(bg)
			}
			fg := mixRGB(bg, targetFg, textAlpha)
			b.WriteString("\x1b[1m" + bgRGB(bg) + fgRGB(fg))
			b.WriteRune(ch)
		}
		b.WriteString("\x1b[0m")
		return b.String()
	}

	return [statusButtonH]string{
		renderCapRow(0, '▄', 0),
		renderMidRow(),
		renderCapRow(2, '▀', math.Pi),
	}
}

func highlightRow(line string, width int) string {
	line = clipLine(line, width)
	if vw := termWidth(line); vw < width {
		line += strings.Repeat(" ", width-vw)
	}
	if selReplacer == nil {
		return line
	}
	return selReset + selReplacer.Replace(line) + "\x1b[0m"
}

func applyCanvas(content string, w, h int) string {
	if !canvasEnabled || canvasReplacer == nil {
		return content
	}

	lines := strings.Split(content, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	if h > 0 && len(lines) > h {
		lines = lines[:h]
	}

	out := make([]string, len(lines))
	for i, line := range lines {
		if vw := termWidth(line); vw < w {
			line += strings.Repeat(" ", w-vw)
		}
		out[i] = canvasBase + canvasReplacer.Replace(line) + "\x1b[0m"
	}
	return strings.Join(out, "\n")
}
