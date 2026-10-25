package tui

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/rednafi/rexdiff/diff"
)

// boxShows reports whether the comment box of m shows in the main pane.
func boxShows(m model) bool {
	p, h := m.cur(), m.mainHeight()
	_, rowAt := m.mainLines(h, nil)
	i := slices.Index(rowAt, p.end(p.cur))
	return m.mode == modeComment && i >= 0 && i+1+lipgloss.Height(m.editor.View()) <= h
}

// longFileModel returns a test model that shows long.go in the Files tab.
func longFileModel(t *testing.T) model {
	t.Helper()
	m, dir := newTestModel(t)
	longFile(t, dir)
	m = press(t, m, "2")
	m.reloadFiles()
	m.jumpToFile("long.go", diff.SideNew, 1)
	if m.viewer.path != "long.go" {
		t.Fatalf("the Files tab shows %q, want long.go", m.viewer.path)
	}
	return m
}

// TestEditorWidth checks that the width of the comment box follows the
// width of the main pane.
func TestEditorWidth(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "z", "]", "c")
	want := m.editor
	want.SetWidth(max(m.mainWidth()-8, 20))
	if m.editor.Width() != want.Width() {
		t.Errorf("box width %d with a hidden list, want %d", m.editor.Width(), want.Width())
	}
}

// TestGutterClickShowsBox checks that a click on the line numbers of the
// last row on the screen scrolls the comment box into view.
func TestGutterClickShowsBox(t *testing.T) {
	m := longFileModel(t)
	y := m.mainY() + m.mainHeight() - 1
	m = step(t, m, tea.MouseClickMsg{Button: tea.MouseLeft, X: m.mainX() + gutter, Y: y})
	m = step(t, m, tea.MouseReleaseMsg{Button: tea.MouseLeft, X: m.mainX() + gutter, Y: y})
	if !boxShows(m) {
		t.Errorf("after a click on the last row, mode %v, view:\n%s\nwant the box", m.mode, view(m))
	}
}

// TestTypingShowsBox checks that a key in the comment box scrolls the box
// into view.
func TestTypingShowsBox(t *testing.T) {
	m := longFileModel(t)
	m = press(t, m, ":", "8", "0", "enter", "c")
	m.viewer.top = 0
	if m = press(t, m, "x"); !boxShows(m) {
		t.Errorf("after a key in the box, view:\n%s\nwant the box", view(m))
	}
}

// TestListEditNeedsLine checks that e in the comment list opens no box if
// the pane does not show the line of the comment. Here the file of the
// comment leaves the changes, so the jump shows it in the Files tab.
func TestListEditNeedsLine(t *testing.T) {
	m, dir := newTestModel(t)
	m = press(t, m, "]", "c", "h", "i", "enter")
	write(t, dir, "a.go", "package a\n\nfunc A() int {\n\treturn 1\n}\n")
	m.reload()
	m.status = ""
	if m = press(t, m, "l", "e"); m.mode == modeComment || m.status == "" {
		t.Errorf("e on a comment whose line is gone: mode %v, status %q; want no box, a flash",
			m.mode, m.status)
	}
}
