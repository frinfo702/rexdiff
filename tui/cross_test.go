package tui

import (
	"testing"

	"github.com/rednafi/rexdiff/diff"
	"github.com/rednafi/rexdiff/internal/testgit"
)

// twoFiles returns a model whose Changes tab has a.go and b.go, each with
// one block of changes. The cursor is on the change of a.go.
func twoFiles(t *testing.T) model {
	t.Helper()
	m, dir := newTestModel(t)
	testgit.WriteFile(t, dir, "b.go", "package b\n")
	m.reload()
	if len(m.files) != 2 {
		t.Fatalf("got %d changed files, want 2", len(m.files))
	}
	return press(t, m, "]")
}

// onChange tells if the cursor of the Changes tab is on a changed row.
func onChange(m model) bool {
	k := m.changes.rows[m.changes.cur].Kind
	return k == diff.KindInsert || k == diff.KindDelete || k == diff.KindChange
}

// TestHunkCrossing checks that ] past the last change arms a crossing, and
// a second ] crosses, as in herdr-reviewr. [ does the same backward.
func TestHunkCrossing(t *testing.T) {
	m := twoFiles(t)
	cur := m.changes.cur
	m = press(t, m, "]")
	if m.armed != 1 || m.changeSel != 0 || m.changes.cur != cur {
		t.Fatalf("] at the last change: armed %d, file %d, cursor %d; want 1, 0, %d",
			m.armed, m.changeSel, m.changes.cur, cur)
	}
	if !shows(m, "] next file") {
		t.Errorf("armed: view got:\n%s\nwant the hint \"] next file\"", view(m))
	}
	m = press(t, m, "]")
	if m.armed != 0 || m.changeSel != 1 || !onChange(m) {
		t.Fatalf("] again: armed %d, file %d; want a crossing to b.go on its change",
			m.armed, m.changeSel)
	}

	if m = press(t, m, "["); m.armed != -1 || m.changeSel != 1 || !shows(m, "[ prev file") {
		t.Fatalf("[ at the first change: armed %d, file %d; want -1, 1", m.armed, m.changeSel)
	}
	if m = press(t, m, "["); m.changeSel != 0 || !onChange(m) {
		t.Errorf("[ again: file %d, want a.go on its change", m.changeSel)
	}
}

// TestHunkCrossingDisarms checks that another key drops an armed crossing.
func TestHunkCrossingDisarms(t *testing.T) {
	m := press(t, twoFiles(t), "]", "j")
	if m.armed != 0 {
		t.Fatalf("] then j: armed %d, want 0", m.armed)
	}
	if m = press(t, m, "k", "]"); m.armed != 1 || m.changeSel != 0 {
		t.Errorf("] after j: armed %d, file %d; want an arm, not a crossing",
			m.armed, m.changeSel)
	}
	// A [ after a ] arms backward, if there is a file that way.
	if m = press(t, m, "["); m.armed != 0 {
		t.Errorf("[ after ]: armed %d, want no arm with no file above", m.armed)
	}
	// A selection holds the cursor.
	cur := m.changes.cur
	if m = press(t, m, "v", "]", "]"); m.changeSel != 0 || m.changes.cur != cur || m.armed != 0 {
		t.Errorf("] with a selection: file %d, cursor %d, armed %d; want no move",
			m.changeSel, m.changes.cur, m.armed)
	}
}

// TestEscLadder checks that esc drops one layer with each press: the
// selection, the armed crossing, then the list of all keys.
func TestEscLadder(t *testing.T) {
	m := press(t, twoFiles(t), "?", "]")
	if !m.help.ShowAll || m.armed != 1 {
		t.Fatalf("? ]: all keys %v, armed %d; want both", m.help.ShowAll, m.armed)
	}
	if m = press(t, m, "esc"); m.armed != 0 || !m.help.ShowAll {
		t.Errorf("esc: armed %d, all keys %v; want 0, true", m.armed, m.help.ShowAll)
	}
	if m = press(t, m, "esc"); m.help.ShowAll {
		t.Error("esc again: the list of all keys stays, want it closed")
	}

	m = press(t, m, "?", "v")
	if m = press(t, m, "esc"); m.changes.anchor != -1 || !m.help.ShowAll {
		t.Errorf("esc with a selection: anchor %d, all keys %v; want -1, true",
			m.changes.anchor, m.help.ShowAll)
	}
	if m = press(t, m, "esc"); m.help.ShowAll {
		t.Error("esc again: the list of all keys stays, want it closed")
	}
}
