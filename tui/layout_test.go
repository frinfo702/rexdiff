package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rednafi/rexdiff/diff"
	"github.com/rednafi/rexdiff/internal/testgit"
)

// TestTinyStackedList checks that a body of 3 rows with the list below
// fits the screen, and that the list then takes no clicks.
func TestTinyStackedList(t *testing.T) {
	t.Parallel()
	for _, pos := range []position{positionBottom, positionTop} {
		m, _ := newTestModel(t)
		m.listPos = pos
		m = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 6})
		if h := m.bodyHeight(); h != 3 {
			t.Fatalf("%s: body height %d, want 3", pos, h)
		}
		lines := strings.Split(view(m), "\n")
		if last := lines[len(lines)-1]; !strings.Contains(last, "comment") {
			t.Errorf("%s: the last line is %q, want the footer", pos, last)
		}
		m = step(t, m, tea.MouseClickMsg{Button: tea.MouseLeft, X: 5, Y: m.mainY() + 1})
		if m.focusList {
			t.Errorf("%s: a click on the main pane gives the focus to the list", pos)
		}
	}
}

// TestListDefaultWidth checks that the file list at a side takes 32% of
// the width if file_list_width is not set, as in herdr-reviewr.
func TestListDefaultWidth(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t)
	if got := m.sideWidth(); got != 120*32/100 {
		t.Errorf("120 columns: list width %d, want %d", got, 120*32/100)
	}
	if m = step(t, m, tea.WindowSizeMsg{Width: 200, Height: 40}); m.sideWidth() != 64 {
		t.Errorf("200 columns: list width %d, want 64", m.sideWidth())
	}
	cfg, err := parseConfig(`{"file_list_width": 40}`)
	if err != nil {
		t.Fatal(err)
	}
	m = newModel(t.Context(), Options{Repo: m.repo}, cfg)
	if got := m.sideWidth(); got != 40 {
		t.Errorf("file_list_width 40: list width %d, want 40", got)
	}
}

// TestMarkdownNotRerendered checks that a tick does not render the
// markdown of the Files tab again if the file does not change.
func TestMarkdownNotRerendered(t *testing.T) {
	t.Parallel()
	m, dir := newTestModel(t)
	testgit.WriteFile(t, dir, "notes.md", "# Title\n\nSome text.\n")
	m = press(t, m, "2")
	m.reloadFiles()
	m.jumpToFile("notes.md", diff.SideNew, 1)
	m = press(t, m, "m")
	if !m.rendered || len(m.viewer.lines) == 0 {
		t.Fatal("m: no rendered markdown")
	}
	m.viewer.lines[0] = "sentinel"
	if m = step(t, m, tickMsg{}); m.viewer.lines[0] != "sentinel" {
		t.Error("a tick with no change renders the markdown again")
	}
}
