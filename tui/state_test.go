package tui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/rednafi/rexdiff/internal/testgit"
)

// TestGrepKeepsFileCursor checks that the end of a code search does not
// move the cursor of the file matches.
func TestGrepKeepsFileCursor(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "/")
	m, cmd := typeQuery(t, m, "go")
	m.overlay.down()
	if m.overlay.cur != 1 {
		t.Fatalf("cursor %d after a move, want 1", m.overlay.cur)
	}
	if m = step(t, m, findGrep(cmd())); m.overlay.cur != 1 {
		t.Errorf("after the code search: cursor %d, want 1", m.overlay.cur)
	}
}

// TestClickDisarms checks that a click or a pick of a file drops an armed
// crossing.
func TestClickDisarms(t *testing.T) {
	m := press(t, twoFiles(t), "]")
	if m.armed != 1 {
		t.Fatalf("armed %d, want 1", m.armed)
	}
	if m = step(t, m, tea.MouseClickMsg{Button: tea.MouseLeft, X: m.mainX() + 5, Y: m.mainY()}); m.armed != 0 {
		t.Errorf("after a click: armed %d, want 0", m.armed)
	}
	m.armed = 1
	if m.pickFile(1); m.armed != 0 {
		t.Errorf("after a pick: armed %d, want 0", m.armed)
	}
}

// TestFocusShowsList checks that tab shows a hidden file list and gives it
// the focus, as in herdr-reviewr.
func TestFocusShowsList(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "z")
	if m = step(t, m, tabKey); m.listHidden || !m.focusList {
		t.Errorf("tab with a hidden list: hidden %v, focus %v; want shown, focused",
			m.listHidden, m.focusList)
	}
}

// TestOverlayClickAboveRows checks that a click above the rows that show
// picks no row.
func TestOverlayClickAboveRows(t *testing.T) {
	m, dir := newTestModel(t)
	for i := range 12 {
		testgit.Git(t, dir, "branch", fmt.Sprintf("b%02d", i))
	}
	m = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 16})
	m = press(t, m, "B")
	for range 10 {
		m.overlay.down()
	}
	_, _, y, _ := m.overlayBox()
	cur := m.overlay.cur
	title := tea.MouseClickMsg{Button: tea.MouseLeft, X: m.w / 2, Y: y + 1}
	m = step(t, m, title)
	if m.overlay == nil {
		t.Fatal("a click on the title closes the overlay")
	}
	if m.overlay.cur != cur {
		t.Errorf("a click on the title: cursor %d, want %d", m.overlay.cur, cur)
	}
}

// TestScopeClearsSelection checks that a change of the scope clears the
// selection and the armed crossing.
func TestScopeClearsSelection(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "]", "v")
	m.armed = 1
	m.setScope(ScopeBranch)
	if m.changes.anchor != -1 || m.armed != 0 || m.changes.cur != 0 {
		t.Errorf("after a change of the scope: anchor %d, armed %d, cursor %d; want -1, 0, 0",
			m.changes.anchor, m.armed, m.changes.cur)
	}
}
