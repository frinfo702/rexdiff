package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// quitPrompt returns a model with one comment and the prompt before a
// quit open.
func quitPrompt(t *testing.T) model {
	t.Helper()
	m, _ := newTestModel(t)
	m = press(t, m, "]", "c", "h", "i", "enter", "q")
	if m.overlay == nil || m.overlay.kind != overlayQuit {
		t.Fatal("q with a comment: no quit prompt, want the prompt")
	}
	return m
}

// quits tells if cmd quits the program.
func quits(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// TestQuitPrompt checks the keys of the prompt before a quit. They are
// the keys of herdr-reviewr.
func TestQuitPrompt(t *testing.T) {
	m := quitPrompt(t)
	const hint = "Q quit (1 pending) · esc cancel · s send 1 · y copy"
	if !shows(m, hint) {
		t.Errorf("the prompt got:\n%s\nwant %q", view(m), hint)
	}

	// A held q does not quit, and the prompt stays open.
	for range 3 {
		next, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
		if m = *next.(*model); quits(cmd) || m.overlay == nil {
			t.Fatalf("q in the prompt: quit %v, overlay %v; want no quit, the prompt open",
				quits(cmd), m.overlay)
		}
	}
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'Q', Text: "Q"}); !quits(cmd) {
		t.Error("Q in the prompt: no quit, want a quit")
	}

	// Any other key closes the prompt and keeps the comment.
	for _, k := range []string{"j", "esc", "enter", "n"} {
		m := press(t, quitPrompt(t), k)
		if m.overlay != nil || m.mode != modeNormal || m.comments.Len() != 1 {
			t.Errorf("%q in the prompt: overlay %v, %d comments; want it closed, 1",
				k, m.overlay, m.comments.Len())
		}
	}
}

// TestQuitPromptSendCopy checks that s and y close the prompt and send or
// copy the comments.
func TestQuitPromptSendCopy(t *testing.T) {
	m := press(t, quitPrompt(t), "s")
	if m.overlay != nil || !strings.Contains(m.status, "not inside Rex") {
		t.Errorf("s in the prompt: overlay %v, status %q; want it closed, a send",
			m.overlay, m.status)
	}

	fakeClipboard(t, false)
	m = stepRun(t, quitPrompt(t), tea.KeyPressMsg{Code: 'y', Text: "y"})
	if m.overlay != nil || m.status != "copied 1 comment" || m.comments.Len() != 0 {
		t.Errorf("y in the prompt: overlay %v, status %q, %d comments; "+
			"want it closed, a copy, 0", m.overlay, m.status, m.comments.Len())
	}
}

// TestNoCtrlCHomeEnd checks that ctrl+c, home, and end do nothing, as in
// herdr-reviewr.
func TestNoCtrlCHomeEnd(t *testing.T) {
	m, _ := newTestModel(t)
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); quits(cmd) {
		t.Error("ctrl+c: a quit, want nothing")
	}
	m = press(t, m, "j")
	cur := m.changes.cur
	if m = press(t, m, "home", "end"); m.changes.cur != cur {
		t.Errorf("home and end: the cursor moves from %d to %d, want no move",
			cur, m.changes.cur)
	}
}

// TestQuitDiscardInert checks that Q does nothing outside the prompt
// before a quit, as in herdr-reviewr.
func TestQuitDiscardInert(t *testing.T) {
	m, _ := newTestModel(t)
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'Q', Text: "Q"}); quits(cmd) {
		t.Error("Q with no comments: a quit, want nothing")
	}
	m = press(t, m, "]", "c", "h", "i", "enter")
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'Q', Text: "Q"})
	if m = *next.(*model); quits(cmd) || m.comments.Len() != 1 || m.overlay != nil {
		t.Errorf("Q with a comment: quit %v, %d comments, overlay %v; want nothing",
			quits(cmd), m.comments.Len(), m.overlay)
	}
}
