package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestKeybindingErrors checks the errors of the keybindings setting. The
// messages are the same as in herdr-reviewr.
func TestKeybindingErrors(t *testing.T) {
	const names = "a list of keys, each a character or a named key " +
		"(left, right, up, down, pageup, pagedown), alone or after ctrl+ or alt+"
	tests := []struct{ text, want string }{
		{`{"comment": ["v"]}`, "config: invalid value for keybindings: `v` is bound to both `select` and `comment`"},
		{`{"comment": ["c", "c"]}`, "config: invalid value for keybindings: `c` is bound twice to `comment`"},
		{`{"refresh": ["ctrl+f"]}`, "config: invalid value for keybindings: `ctrl+f` is bound to both `find` and `refresh`"},
		{`{"wrap": ["|"]}`, "config: invalid value for keybindings: `|` is bound to both `wrap` and `split`"},
		{`{"next-file": ["pageup"]}`, "config: invalid value for keybindings: `pageup` is bound to both `next-file` and `page-up`"},
		{`{"page-up": ["PageUp"]}`, `config: invalid value for keybindings.page-up: expected ` + names + `, not "PageUp"`},
		{`{"send": ["ss"]}`, `keybindings.send: expected ` + names + `, not "ss"`},
		{`{"send": [" "]}`, `keybindings.send: expected ` + names + `, not " "`},
		{`{"send": ["ctrl+"]}`, `keybindings.send: expected ` + names + `, not "ctrl+"`},
		{`{"send": [""]}`, `keybindings.send: expected ` + names + `, not ""`},
		{`{"send": []}`, `keybindings.send: expected ` + names + `, not "[]"`},
	}
	for _, tt := range tests {
		_, err := parseConfig(`{"keybindings": ` + tt.text + `}`)
		if !errorHas(err, tt.want) {
			t.Errorf("keybindings %s: error %v, want %q", tt.text, err, tt.want)
		}
	}
}

// TestKeybindingsValid checks settings with no error: a swap of two keys,
// and the names of keys.
func TestKeybindingsValid(t *testing.T) {
	for _, text := range []string{
		`{"comment": ["v"], "select": ["c"]}`,
		`{"page-up": ["pageup", "alt+up"], "page-down": ["pgdown"]}`,
		`{"find": ["alt+f"], "send": ["é"]}`,
	} {
		if _, err := parseConfig(`{"keybindings": ` + text + `}`); err != nil {
			t.Errorf("keybindings %s: %v, want no error", text, err)
		}
	}
}

// TestKeybindingPageUp checks that the herdr-reviewr name pageup binds the
// page up key.
func TestKeybindingPageUp(t *testing.T) {
	m, _ := newTestModel(t)
	cfg, err := parseConfig(`{"keybindings": {"page-up": ["pageup"], "half-up": ["pgup"]}}`)
	if err == nil {
		t.Fatal("pageup and pgup on two actions: no error, want an error")
	}
	cfg, err = parseConfig(`{"keybindings": {"page-up": ["pageup"]}}`)
	if err != nil {
		t.Fatal(err)
	}
	m.keys = defaultKeys()
	m.keys.applyConfig(cfg.Keybindings)
	if m = press(t, m, "j", "j"); m.changes.cur != 2 {
		t.Fatalf("after j j: cursor %d, want 2", m.changes.cur)
	}
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	if m.changes.cur != 0 {
		t.Errorf("pageup bound to page-up: cursor %d, want 0", m.changes.cur)
	}
}
