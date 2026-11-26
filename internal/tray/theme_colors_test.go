package tray

import "testing"

func TestThemeIconColors(t *testing.T) {
	darkOff, darkOn := themeIconColors(themeDark)
	lightOff, lightOn := themeIconColors(themeLight)

	if darkOn != "#ffffff" {
		t.Errorf("на тёмной теме активная иконка должна быть белой, получили %q", darkOn)
	}
	if lightOn != "#1f1f1f" {
		t.Errorf("на светлой теме активная иконка должна быть чёрной, получили %q", lightOn)
	}
	if darkOff == lightOff {
		t.Errorf("неактивные иконки тем должны различаться: %q", darkOff)
	}
	if darkOff == darkOn || lightOff == lightOn {
		t.Error("активное и неактивное состояния должны различаться цветом")
	}

	unknownOff, unknownOn := themeIconColors(themeUnknown)
	if unknownOff == "" {
		t.Error("для неизвестной темы нужен нейтральный цвет")
	}
	if unknownOn != "" {
		t.Errorf("для неизвестной темы активный цвет задаёт акцент, получили %q", unknownOn)
	}
}
