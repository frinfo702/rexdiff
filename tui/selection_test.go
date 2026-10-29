package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// paneCell returns the screen cell of the first needle in the main pane,
// and the row of the pane that holds it.
func paneCell(t *testing.T, m model, needle string) (x, y, row int) {
	t.Helper()
	p := m.cur()
	for i, l := range p.lines {
		plain := ansi.Strip(l)
		if k := strings.Index(plain, needle); k >= 0 {
			return m.mainX() + gutter + ansi.StringWidth(plain[:k]), screenY(t, m, i), i
		}
	}
	t.Fatalf("no %q in the main pane", needle)
	return 0, 0, 0
}

// dragText selects from x1, y1 to x2, y2 with the left button, and returns
// the command of the release.
func dragText(t *testing.T, m model, x1, y1, x2, y2 int) (model, tea.Cmd) {
	t.Helper()
	m = step(t, m, tea.MouseClickMsg{Button: tea.MouseLeft, X: x1, Y: y1})
	m = step(t, m, tea.MouseMotionMsg{Button: tea.MouseLeft, X: x2, Y: y2})
	next, cmd := m.Update(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x2, Y: y2})
	return *next.(*model), cmd
}

// clipboardText returns the text that cmd puts on the clipboard.
func clipboardText(t *testing.T, cmd tea.Cmd) string {
	t.Helper()
	if cmd == nil {
		t.Fatal("no clipboard command")
	}
	return fmt.Sprint(cmd())
}

// TestDragSelectsText checks that a drag over the code selects the cells
// under the pointer and copies them.
func TestDragSelectsText(t *testing.T) {
	m, _ := newTestModel(t)
	x, y, _ := paneCell(t, m, "x := 2")
	end := x + ansi.StringWidth("x := 2") - 1
	m, cmd := dragText(t, m, x, y, end, y)

	if got := clipboardText(t, cmd); got != "x := 2" {
		t.Errorf("clipboard %q, want %q", got, "x := 2")
	}
	if m.changes.text == nil {
		t.Fatal("the selection is gone after the release")
	}
	if got := m.changes.selectedText(); got != "x := 2" {
		t.Errorf("selected text %q, want %q", got, "x := 2")
	}
	if m.status != "copied 6 chars" {
		t.Errorf("status %q, want the count of chars", m.status)
	}
	if !strings.Contains(view(m), "copy selection") {
		t.Errorf("the footer does not show the selection keys:\n%s", view(m))
	}
	if !strings.Contains(m.View().Content, m.textSelectSGR()) {
		t.Error("the selected cells have no selection colour")
	}

	// y copies the selection again, and esc clears it.
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = *next.(*model)
	if got := clipboardText(t, cmd); got != "x := 2" {
		t.Errorf("y copied %q, want %q", got, "x := 2")
	}
	m = press(t, m, "esc")
	if m.changes.text != nil {
		t.Error("esc did not clear the selection")
	}
}

// TestDragSelectsBackwards checks a drag from the end of a word to its
// start, and that a click without a drag selects nothing.
func TestDragSelectsBackwards(t *testing.T) {
	m, _ := newTestModel(t)
	x, y, _ := paneCell(t, m, "x := 2")
	end := x + ansi.StringWidth("x := 2") - 1
	m, cmd := dragText(t, m, end, y, x, y)
	if got := clipboardText(t, cmd); got != "x := 2" {
		t.Errorf("clipboard %q, want %q", got, "x := 2")
	}

	m = click(t, m, x, y)
	if m.changes.text != nil {
		t.Error("a click without a drag kept a selection")
	}
}

// TestDragSelectsAcrossLines checks that a selection copies each line and
// leaves out the spaces at their ends.
func TestDragSelectsAcrossLines(t *testing.T) {
	m, _ := newTestModel(t)
	x, y, _ := paneCell(t, m, "x := 2")
	x2, y2, _ := paneCell(t, m, "return x")
	end := x2 + ansi.StringWidth("return x") - 1
	var cmd tea.Cmd
	m, cmd = dragText(t, m, x, y, end, y2)
	if cmd == nil {
		t.Fatal("no clipboard command")
	}
	want := "x := 2\n    return x"
	if got := m.changes.selectedText(); got != want {
		t.Errorf("selected text %q, want %q", got, want)
	}
}

// TestSelectionKeys checks that a movement key clears the selection, and
// that y copies comments when nothing is selected.
func TestSelectionKeys(t *testing.T) {
	m, _ := newTestModel(t)
	x, y, _ := paneCell(t, m, "x := 2")
	m, _ = dragText(t, m, x, y, x+1, y)
	if m = press(t, m, "j"); m.changes.text != nil {
		t.Error("j did not clear the selection")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = *next.(*model)
	if cmd != nil || !strings.Contains(m.status, "no comments yet") {
		t.Errorf("y with no selection: status %q, command %v", m.status, cmd)
	}
}

// TestDragSelectsSplitSide checks that a drag in the split layout copies
// the code of the side that it starts on, and not the other side.
func TestDragSelectsSplitSide(t *testing.T) {
	m, _ := newTestModel(t)
	m = step(t, m, tea.WindowSizeMsg{Width: 200, Height: 40})
	if !m.splitShown() {
		t.Fatal("a pane 200 columns wide does not show the diff side by side")
	}
	tests := []struct {
		needle, want string
	}{
		{"x := 2", "x := 2"},
		{"return 1", "return 1"},
	}
	for _, tt := range tests {
		x, y, _ := paneCell(t, m, tt.needle)
		end := x + ansi.StringWidth(tt.needle) - 1
		_, cmd := dragText(t, m, x, y, end, y)
		if got := clipboardText(t, cmd); got != tt.want {
			t.Errorf("%q: clipboard %q, want %q", tt.needle, got, tt.want)
		}
	}
}

// TestMarkSelection checks that the selection colour stops at the last
// selected cell, also on a line with no colours.
func TestMarkSelection(t *testing.T) {
	m, _ := newTestModel(t)
	p := &pane{lines: []string{"abcdefgh"}}
	p.text = &textSelection{start: textPoint{row: 0, col: 2}, end: textPoint{row: 0, col: 4}}
	sgr := m.textSelectSGR()
	got := p.markSelection(0, sgr)
	want := "ab" + sgr + "cde" + ansi.ResetStyle + "fgh"
	if got != want {
		t.Errorf("marked line %q, want %q", got, want)
	}
}
