package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseExternalRulesJSONFromPaste(t *testing.T) {
	// JSON, пришедший с маркерами bracketed paste: раньше уходил в ReadFile.
	pasted := "\x1b[200~[{\"domain\":[\"example.com\"],\"outboundTag\":\"proxy\"}]\x1b[201~"
	rules, err := parseExternalRulesJSON(pasted)
	if err != nil {
		t.Fatalf("JSON из вставки не распознан: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("правил %d, ждали 1", len(rules))
	}
}

func TestParseExternalRulesJSONFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")
	if err := os.WriteFile(path, []byte(`[{"domain":["a.b"],"outboundTag":"direct"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	rules, err := parseExternalRulesJSON(path)
	if err != nil {
		t.Fatalf("файл не прочитан: %v", err)
	}
	if len(rules) != 1 || rules[0].Outbound != "direct" {
		t.Fatalf("неожиданные правила: %#v", rules)
	}
}

func TestParseExternalRulesJSONWrapperForm(t *testing.T) {
	rules, err := parseExternalRulesJSON(`{"routing":{"rules":[{"domain":["x.y"],"outboundTag":"proxy"}]}}`)
	if err != nil {
		t.Fatalf("обёртка routing.rules не разобрана: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("правил %d, ждали 1", len(rules))
	}
}
