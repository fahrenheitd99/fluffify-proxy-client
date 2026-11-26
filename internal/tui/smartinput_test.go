package tui

import (
	"testing"

	"xray-tui/internal/config"
)

func newSmartInputModel(t *testing.T) model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	store, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return model{store: store, subForm: subFormState{active: true}}
}

func TestApplySmartInputSubscription(t *testing.T) {
	m := newSmartInputModel(t)
	// URL-кодированная ссылка подписки: раньше падала с «неподдерживаемый протокол».
	out, _ := m.applySmartInput("%68%74%74%70%73%3A%2F%2Fsub.example%2Fabc")
	got := out.(model)
	if len(got.store.Subscriptions) != 1 {
		t.Fatalf("подписок %d, ждали 1 (%q)", len(got.store.Subscriptions), got.notice)
	}
	if got.store.Subscriptions[0].URL != "https://sub.example/abc" {
		t.Fatalf("URL подписки = %q", got.store.Subscriptions[0].URL)
	}
	if !got.busy {
		t.Fatal("после добавления подписки должна идти загрузка")
	}
}

func TestApplySmartInputLink(t *testing.T) {
	m := newSmartInputModel(t)
	link := "vless://11111111-1111-1111-1111-111111111111@a.example:443?type=tcp#node"
	out, _ := m.applySmartInput(link)
	got := out.(model)
	if len(got.store.Profiles) != 1 {
		t.Fatalf("профилей %d, ждали 1 (%q)", len(got.store.Profiles), got.notice)
	}
	if got.store.Profiles[0].Protocol != "vless" {
		t.Fatalf("протокол = %q", got.store.Profiles[0].Protocol)
	}
}

func TestApplySmartInputJSONWithPasteMarkers(t *testing.T) {
	m := newSmartInputModel(t)
	jsonNode := "\x1b[200~{\"protocol\":\"vless\",\"settings\":{\"vnext\":[{\"address\":\"a.example\",\"port\":443}]}}\x1b[201~"
	out, _ := m.applySmartInput(jsonNode)
	got := out.(model)
	if len(got.store.Profiles) != 1 {
		t.Fatalf("профилей %d, ждали 1 (%q)", len(got.store.Profiles), got.notice)
	}
	if got.store.Profiles[0].Type != config.ProfileTypeRaw {
		t.Fatalf("тип профиля = %q, ждали raw", got.store.Profiles[0].Type)
	}
}

func TestApplySmartInputUnknown(t *testing.T) {
	m := newSmartInputModel(t)
	out, _ := m.applySmartInput("совсем не ссылка")
	got := out.(model)
	if !got.noticeErr || got.notice == "" {
		t.Fatal("на непонятный ввод должна быть внятная ошибка")
	}
	if len(got.store.Profiles) != 0 || len(got.store.Subscriptions) != 0 {
		t.Fatal("на ошибке ничего не должно добавляться")
	}
}
