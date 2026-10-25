package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// screenLine returns line y of the screen of m.
func screenLine(m model, y int) string {
	lines := strings.Split(view(m), "\n")
	if y < 0 || y >= len(lines) {
		return ""
	}
	return lines[y]
}

// TestListPositions checks that p moves the file list clockwise, and that
// a list above or below takes a quarter of the body rows.
func TestListPositions(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t)
	want := []position{positionBottom, positionLeft, positionTop, positionRight}
	for _, w := range want {
		m = press(t, m, "p")
		if m.listPos != w {
			t.Fatalf("after p: position %q, want %q", m.listPos, w)
		}
		title := screenLine(m, m.listY())
		if !strings.Contains(title, "Changes (1)") {
			t.Errorf("%s: line %d is %q, want the title of the list", w, m.listY(), title)
		}
		if w.stacked() {
			h := m.bodyHeight()
			if got, want := m.listHeight(h), h*listStackPct/100; got != want {
				t.Errorf("%s: the list takes %d of %d rows, want %d", w, got, h, want)
			}
			if m.mainWidth() != m.w {
				t.Errorf("%s: main width %d, want the screen width %d", w, m.mainWidth(), m.w)
			}
			if m.mainHeight()+m.listHeight(h) != h {
				t.Errorf("%s: main %d + list %d rows, want %d", w, m.mainHeight(), m.listHeight(h), h)
			}
		}
	}
	// A click on the file in the list above the main pane selects it.
	m = press(t, m, "p", "p", "p") // top
	m.focusList = false
	m = click(t, m, 5, m.listY()+1)
	if !m.focusList {
		t.Error("a click on the file in the list above: the list has no focus, want focus")
	}
}

// TestListSizes checks the bounds of the file list at a side and above or
// below the main pane.
func TestListSizes(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t) // 120 columns
	m = press(t, m, "<")
	if got := m.sideWidth(); got != 42 {
		t.Errorf("one grow from 38 columns: width %d, want 42", got)
	}
	for range 20 {
		m = press(t, m, "<")
	}
	if got := m.sideWidth(); got != 120*listMaxSidePct/100 {
		t.Errorf("after many grows: width %d, want %d", got, 120*listMaxSidePct/100)
	}
	for range 30 {
		m = press(t, m, ">")
	}
	if got := m.sideWidth(); got != 120*listMinPct/100 {
		t.Errorf("after many shrinks: width %d, want %d", got, 120*listMinPct/100)
	}

	m = press(t, m, "p") // bottom
	m = press(t, m, "<")
	if m.listPct != listStackPct+listStep {
		t.Errorf("one grow below: share %d%%, want %d%%", m.listPct, listStackPct+listStep)
	}
	for range 20 {
		m = press(t, m, "<")
	}
	if m.listPct != listMaxStackPct {
		t.Errorf("after many grows below: share %d%%, want %d%%", m.listPct, listMaxStackPct)
	}
	for range 20 {
		m = press(t, m, ">")
	}
	if m.listPct != listMinPct {
		t.Errorf("after many shrinks below: share %d%%, want %d%%", m.listPct, listMinPct)
	}
}

// TestListHide checks that z hides the list and focuses the main pane,
// that a hidden list does not move or resize, and that the PR tab does
// not hide the list.
func TestListHide(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t)
	m.focusList = true
	m = press(t, m, "z")
	if !m.listHidden || m.focusList || m.listWidth() != 0 {
		t.Fatalf("after z: hidden %v, list focus %v, width %d; want true, false, 0",
			m.listHidden, m.focusList, m.listWidth())
	}
	w, pos := m.listW, m.listPos
	m = press(t, m, "p", "<")
	if m.listPos != pos || m.listW != w {
		t.Errorf("p and < on a hidden list: %q %d, want %q %d", m.listPos, m.listW, pos, w)
	}
	m = press(t, m, "z")
	if m.listHidden {
		t.Error("after a second z: the list is hidden, want it shown")
	}
	m = press(t, m, "3", "z")
	if m.listHidden {
		t.Error("z on the PR tab hides the list, want no change")
	}
}

// TestResumeListLeft checks that a saved state of an older version, with
// list_left, restores the list at the left.
func TestResumeListLeft(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t)
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"list_left": true, "list_w": 40}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := loadState(path)
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}
	m.restoreSettings(s)
	if m.listPos != positionLeft || m.listW != 40 || m.listPct != listStackPct {
		t.Errorf("restored %q, width %d, share %d; want left, 40, %d",
			m.listPos, m.listW, m.listPct, listStackPct)
	}
}
