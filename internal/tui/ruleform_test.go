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

	m = pressKey(m, tea.KeyDown)
	if m.ruleForm.step != 1 {
		t.Fatalf("step = %d, ждали 1 (выбор типа поля)", m.ruleForm.step)
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
