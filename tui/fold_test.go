package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rednafi/rexdiff/diff"
	"github.com/rednafi/rexdiff/vcs"
)

// newFoldModel returns a model that shows f.txt, a file of 40 lines with
// changes at lines 10 and 30. The diff folds lines 1 to 6, 14 to 26, and
// 34 to 40.
func newFoldModel(t *testing.T) model {
	t.Helper()
	dir := t.TempDir()
	lines := func(upper ...int) string {
		var b strings.Builder
		for i := 1; i <= 40; i++ {
			word := "line"
			if slices.Contains(upper, i) {
				word = "LINE"
			}
			fmt.Fprintf(&b, "%s %d\n", word, i)
		}
		return b.String()
	}
	git(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "f.txt", lines())
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "init")
	write(t, dir, "f.txt", lines(10, 30))
	repo, err := vcs.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(t.Context(), Options{Repo: repo, Layout: LayoutUnified}, defaultConfig())
	m.reload()
	return step(t, m, tea.WindowSizeMsg{Width: 100, Height: 70})
}

// foldsOf returns the number of lines of each fold that hides in p.
func foldsOf(p *pane) []int {
	var out []int
	for _, r := range p.rows {
		if r.Fold.Lines > 0 {
			out = append(out, r.Fold.Lines)
		}
	}
	return out
}

// TestExpandFold checks that → shows the lines of a fold and scrolls
// sideways on a line of code.
func TestExpandFold(t *testing.T) {
	m := newFoldModel(t)
	p := &m.changes
	if got := foldsOf(p); !slices.Equal(got, []int{6, 13, 7}) {
		t.Fatalf("folds = %v, want [6 13 7]", got)
	}
	if !shows(m, "⋯ 13 unmodified lines", "→ show the lines") {
		t.Fatalf("the screen does not show the fold and its key:\n%s", view(m))
	}
	m = press(t, m, "]") // the first change, line 10
	if shows(m, "→ show the lines") {
		t.Error("the footer tells about → on a line of code")
	}
	m = press(t, m, "]", "k", "k", "k", "k") // the second change, then up to its header
	if r := m.changes.rows[m.changes.cur]; r.Fold.Lines != 13 || !shows(m, "→ show the lines") {
		t.Fatalf("the cursor is on %+v, want the middle fold", r)
	}
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	p = &m.changes
	if got := foldsOf(p); !slices.Equal(got, []int{6, 7}) {
		t.Errorf("after →, folds = %v, want [6 7]", got)
	}
	if r := p.rows[p.cur]; r.New != 14 {
		t.Errorf("after →, the cursor is on %+v, want line 14", r)
	}
	for n := 14; n <= 26; n++ {
		if p.rowOf(diff.SideNew, n) < 0 {
			t.Errorf("line %d does not show", n)
		}
	}
	// On a line of code, → scrolls.
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.changes.xoff == 0 || !slices.Equal(foldsOf(&m.changes), []int{6, 7}) {
		t.Errorf("→ on code: xoff %d, folds %v; want a scroll and no change",
			m.changes.xoff, foldsOf(&m.changes))
	}
}

// TestGotoLineInFold checks that : with a line in a fold shows the fold and
// moves the cursor to the line.
func TestGotoLineInFold(t *testing.T) {
	m := newFoldModel(t)
	m = press(t, m, ":")
	m = step(t, m, tea.KeyPressMsg{Text: "20", Code: '2'})
	m = press(t, m, "enter")
	p := &m.changes
	if r := p.rows[p.cur]; r.New != 20 {
		t.Errorf("the cursor is on %+v, want line 20", r)
	}
	if got := foldsOf(p); !slices.Equal(got, []int{6, 7}) {
		t.Errorf("folds = %v, want [6 7]", got)
	}
}

// TestExpandKeepsPlaces checks the cursor, the selection, and the hits of
// a find after a fold shows.
func TestExpandKeepsPlaces(t *testing.T) {
	m := newFoldModel(t)
	m.changes.cur = m.changes.rowOf(diff.SideNew, 29)
	m = press(t, m, "v") // the selection starts on line 29
	m = step(t, m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m = step(t, m, tea.KeyPressMsg{Text: "line 3", Code: 'l'})
	m = press(t, m, "enter")
	if !m.find.active() {
		t.Fatal("the find has no hits")
	}
	m.changes.cur = 0 // the fold at the top
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	p := &m.changes
	if r := p.rows[p.cur]; r.New != 1 {
		t.Errorf("the cursor is on %+v, want line 1", r)
	}
	if r := p.rows[p.anchor]; r.New != 29 {
		t.Errorf("the selection starts on %+v, want line 29", r)
	}
	if len(m.find.hits) == 0 {
		t.Error("the find has no hits after the fold shows")
	}
	for _, h := range m.find.hits {
		if l := strings.ToLower(ansi.Strip(p.lines[h])); !strings.Contains(l, "line 3") {
			t.Errorf("a hit is on %q, want a line with line 3", l)
		}
	}
}

// TestFoldStaysOpen checks that an expanded fold stays open after edits of
// the agent and closes after a commit.
func TestFoldStaysOpen(t *testing.T) {
	m := newFoldModel(t)
	m.changes.cur = slices.IndexFunc(m.changes.rows, func(r diff.Row) bool { return r.Fold.Lines == 13 })
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	dir := m.repo.Root()
	edit := func(f func(string) string) {
		b, err := os.ReadFile(filepath.Join(dir, "f.txt"))
		if err != nil {
			t.Fatal(err)
		}
		write(t, dir, "f.txt", f(string(b)))
		m.reload()
	}
	edit(func(s string) string { return "new\n" + s })
	if got := foldsOf(&m.changes); !slices.Equal(got, []int{3, 7}) {
		t.Errorf("after a line above, folds = %v, want [3 7]", got)
	}
	edit(func(s string) string { return strings.Replace(s, "line 20\n", "LINE 20\n", 1) })
	if got := foldsOf(&m.changes); !slices.Equal(got, []int{3, 7}) {
		t.Errorf("after a change in the fold, folds = %v, want [3 7]", got)
	}
	// An undo of the change at line 31 makes one fold of old lines 14 to
	// 40. It overlaps the open range, so it shows.
	edit(func(s string) string { return strings.Replace(s, "LINE 30\n", "line 30\n", 1) })
	if got := foldsOf(&m.changes); !slices.Equal(got, []int{3}) {
		t.Errorf("after an undo next to the fold, folds = %v, want [3]", got)
	}
	// After the commit, the file has 41 lines. Changes at lines 12 and 26
	// make a fold of old lines 16 to 22, inside the open range.
	git(t, dir, "commit", "-qam", "agent")
	edit(func(s string) string {
		lines := strings.Split(s, "\n")
		lines[11], lines[25] = "changed", "changed"
		return strings.Join(lines, "\n")
	})
	if got := foldsOf(&m.changes); !slices.Equal(got, []int{8, 7, 12}) {
		t.Errorf("after a commit, folds = %v, want [8 7 12]", got)
	}
}

// TestFoldsSurviveRestart checks that the expanded folds and the cursor
// line come back after a restart for an update.
func TestFoldsSurviveRestart(t *testing.T) {
	m := newFoldModel(t)
	m.changes.cur = slices.IndexFunc(m.changes.rows, func(r diff.Row) bool { return r.Fold.Lines == 13 })
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	m.changes.cur = m.changes.rowOf(diff.SideNew, 20)
	path, err := m.saveState()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := loadState(path)
	if err != nil {
		t.Fatal(err)
	}
	m2 := newModel(t.Context(), Options{Repo: m.repo, Layout: LayoutUnified}, defaultConfig())
	m2.restoreSettings(saved)
	m2.reload()
	m2.restorePlaces(saved)
	if got := foldsOf(&m2.changes); !slices.Equal(got, []int{6, 7}) {
		t.Errorf("after the restart, folds = %v, want [6 7]", got)
	}
	if r := m2.changes.rows[m2.changes.cur]; r.New != 20 {
		t.Errorf("after the restart, the cursor is on %+v, want line 20", r)
	}
}

// TestClickFold checks that a click on a fold shows its lines, also in the
// split layout and with wrap.
func TestClickFold(t *testing.T) {
	for _, keys := range [][]string{nil, {"|"}, {"w"}} {
		m := press(t, newFoldModel(t), keys...)
		tail := slices.IndexFunc(m.changes.rows, func(r diff.Row) bool { return r.Fold.Lines == 7 })
		m = click(t, m, m.mainX()+10, screenY(t, m, tail))
		if got := foldsOf(&m.changes); !slices.Equal(got, []int{6, 13}) {
			t.Errorf("keys %v: after the click, folds = %v, want [6 13]", keys, got)
		}
		if m.changes.rowOf(diff.SideNew, 40) < 0 {
			t.Errorf("keys %v: line 40 does not show", keys)
		}
	}
}

// TestFoldKeepsComment checks that a fold with a comment on one of its
// lines shows, also after a new scope closes the expanded folds.
func TestFoldKeepsComment(t *testing.T) {
	m := newFoldModel(t)
	m.changes.cur = slices.IndexFunc(m.changes.rows, func(r diff.Row) bool { return r.Fold.Lines == 13 })
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	m.changes.cur = m.changes.rowOf(diff.SideNew, 20)
	m = press(t, m, "c")
	m = step(t, m, tea.KeyPressMsg{Text: "why", Code: 'w'})
	m = press(t, m, "enter")
	if m.comments.Len() != 1 {
		t.Fatalf("%d comments, want 1", m.comments.Len())
	}
	m = press(t, m, "b", "u") // the branch scope, then back
	if m.folds != nil {
		t.Errorf("a new scope keeps the expanded folds %v", m.folds)
	}
	if m.changes.rowOf(diff.SideNew, 20) < 0 || !shows(m, "why") {
		t.Errorf("the commented line 20 does not show:\n%s", view(m))
	}
	if got := foldsOf(&m.changes); !slices.Equal(got, []int{6, 7}) {
		t.Errorf("folds = %v, want [6 7]", got)
	}
}
