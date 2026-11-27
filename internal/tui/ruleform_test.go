package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"xray-tui/internal/config"
)

func newRuleFormModel(t *testing.T) model {
	t.Helper()
	m := newSmartInputModel(t)
	m.subForm = subFormState{}
	m.tab = tabSettings
	m.routingView = routingProfileEdit
	m.routingEditIdx = -1
	m.ruleForm = ruleFormState{active: true, editIdx: -1}
	return m
}

func typeKey(m model, text string) model {
	out, _ := m.handleRoutingKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
	return out.(model)
}

func pressKey(m model, key tea.KeyType) model {
	out, _ := m.handleRoutingKey(tea.KeyMsg{Type: key})
	return out.(model)
}

// Регрессия: клавиши формы правил должны попадать в handleRuleFormKey,
// а не в экран под формой (иначе ввод текста и Esc не работают).
func TestRuleFormReceivesKeys(t *testing.T) {
	m := newRuleFormModel(t)

	m = typeKey(m, "block-ads")
	if m.ruleForm.name != "block-ads" {
		t.Fatalf("имя правила = %q, текст не дошёл до формы", m.ruleForm.name)
	}

	// Новое правило: шаг 1 — поле JSON, шаг 2 — выбор типа поля.
	m = pressKey(m, tea.KeyDown)
	if m.ruleForm.step != 1 {
		t.Fatalf("step = %d, ждали 1 (поле JSON)", m.ruleForm.step)
	}

	m = pressKey(m, tea.KeyDown)
	if m.ruleForm.step != 2 {
		t.Fatalf("step = %d, ждали 2 (выбор типа поля)", m.ruleForm.step)
	}

	m = pressKey(m, tea.KeyRight)
	if m.ruleForm.matchType != 1 {
		t.Fatalf("matchType = %d, стрелка вправо не переключила поле", m.ruleForm.matchType)
	}

	m = pressKey(m, tea.KeyDown)
	m = typeKey(m, "example.com")
	if m.ruleForm.value != "example.com" {
		t.Fatalf("значение = %q, ввод в форму потерян", m.ruleForm.value)
	}

	m = pressKey(m, tea.KeyEsc)
	if m.ruleForm.active {
		t.Fatal("Esc должен закрывать форму правила")
	}
}

func TestRuleFormCommitsRule(t *testing.T) {
	m := newRuleFormModel(t)
	m.store.Routing.Profiles = append(m.store.Routing.Profiles, config.RoutingProfile{
		ID:            config.NewID(),
		Name:          "test",
		GeoProviderID: config.DefaultGeoProviderID,
	})
	m.routingEditIdx = 0

	m.ruleForm.name = "block-ads"
	m.ruleForm.matchType = 0 // domain
	m.ruleForm.value = "ads.example.com"
	m.ruleForm.outbound = 2 // block

	out, _ := m.handleRoutingKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	got := out.(model)
	if got.ruleForm.active {
		t.Fatal("после Ctrl+S форма должна закрыться")
	}
	rules := got.store.Routing.Profiles[0].Rules
	var added *config.RoutingRule
	for i := range rules {
		if rules[i].Name == "block-ads" {
			added = &rules[i]
			break
		}
	}
	if added == nil {
		t.Fatalf("правило block-ads не сохранено (всего правил %d)", len(rules))
	}
	if added.Outbound != "block" {
		t.Fatalf("outbound = %q, ждали block", added.Outbound)
	}
	if len(added.Domains) != 1 || added.Domains[0] != "ads.example.com" {
		t.Fatalf("домены правила: %#v", added.Domains)
	}
}

// Новое правило с заполненным полем JSON: правила импортируются пачкой,
// ручные поля (тип/значения) при этом игнорируются.
func TestRuleFormImportsJSON(t *testing.T) {
	m := newRuleFormModel(t)
	m.store.Routing.Profiles = append(m.store.Routing.Profiles, config.RoutingProfile{
		ID:            config.NewID(),
		Name:          "test",
		GeoProviderID: config.DefaultGeoProviderID,
	})
	m.routingEditIdx = len(m.store.Routing.Profiles) - 1

	m.ruleForm.jsonRules = `[{"outboundTag":"block","domain":["ads.example.com"]},{"outboundTag":"direct","ip":["10.0.0.0/8"],"remarks":"LAN"}]`

	out, _ := m.handleRoutingKey(tea.KeyMsg{Type: tea.KeyCtrlS})
	got := out.(model)
	if got.ruleForm.active {
		t.Fatal("после импорта форма должна закрыться")
	}
	rules := got.store.Routing.Profiles[m.routingEditIdx].Rules
	if len(rules) != 2 {
		t.Fatalf("импортировано %d правил, ждали 2", len(rules))
	}
	if rules[0].Outbound != "block" || len(rules[0].Domains) != 1 {
		t.Fatalf("первое правило: %#v", rules[0])
	}
	if rules[1].Name != "LAN" || rules[1].Outbound != "direct" {
		t.Fatalf("второе правило: %#v", rules[1])
	}
}

// Новый профиль роутинга создаётся ПУСТЫМ: дефолтные правила — только
// у встроенного «Основного».
func TestNewRoutingProfileIsEmpty(t *testing.T) {
	m := newRuleFormModel(t)
	base := len(m.store.Routing.Profiles)

	// Форма правила перекрывает форму профиля — закрываем её.
	m.ruleForm = ruleFormState{}
	m.profileForm = profileFormState{active: true, name: "Пустой"}
	out, _ := m.handleRoutingKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := out.(model)

	if len(got.store.Routing.Profiles) != base+1 {
		t.Fatalf("профиль не создан (%d → %d)", base, len(got.store.Routing.Profiles))
	}
	created := got.store.Routing.Profiles[len(got.store.Routing.Profiles)-1]
	if created.Name != "Пустой" {
		t.Fatalf("создан %q, ждали «Пустой»", created.Name)
	}
	if len(created.Rules) != 0 {
		t.Fatalf("новый профиль должен быть без правил, а там %d", len(created.Rules))
	}
	// Встроенный «Основной» остаётся с дефолтными правилами.
	if len(got.store.Routing.Profiles[0].Rules) == 0 {
		t.Fatal("у встроенного профиля правила не должны пропасть")
	}
}
