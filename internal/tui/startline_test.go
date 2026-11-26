package tui

import (
	"strings"
	"testing"
)

func TestBuildStartLine(t *testing.T) {
	got := buildStartLine(
		`C:\Windows\System32\cmd.exe`,
		`C:\Users\me\Fluffify`,
		`C:\Users\me\Fluffify\xray-tui.exe`,
	)
	want := `"C:\Windows\System32\cmd.exe" /c start "Fluffify" /D "C:\Users\me\Fluffify" "C:\Users\me\Fluffify\xray-tui.exe" tui`
	if got != want {
		t.Fatalf("buildStartLine:\n получили %s\n ждали    %s", got, want)
	}
	if strings.Contains(strings.ToLower(got), `fluffify.exe`) {
		t.Fatalf("Fluffify не должен превращаться в имя программы: %s", got)
	}
	if !strings.Contains(got, `start "Fluffify" /D`) {
		t.Fatalf("заголовок и /D должны идти сразу после start: %s", got)
	}
}

func TestBuildStartLineRootDir(t *testing.T) {
	got := buildStartLine(`C:\Windows\System32\cmd.exe`, `C:\`, `C:\xray-tui.exe`)
	want := `"C:\Windows\System32\cmd.exe" /c start "Fluffify" /D "C:\." "C:\xray-tui.exe" tui`
	if got != want {
		t.Fatalf("buildStartLine для корня диска:\n получили %s\n ждали    %s", got, want)
	}
}
