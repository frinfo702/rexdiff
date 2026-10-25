package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/rednafi/rexdiff/diff"
)

// find opens the find band and types q.
func find(t *testing.T, m model, q string) model {
	t.Helper()
	m = step(t, m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	if m.mode != modeFind {
		t.Fatalf("ctrl+f: mode %v, want the find band", m.mode)
	}
	for _, r := range q {
		m = step(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// curRow returns the row at the cursor of the current pane.
func curRow(m model) diff.Row {
	p := m.cur()
	return p.rows[p.cur]
}

// TestFindSmartCase checks that a query in lower case ignores case, and a
// query with an upper case letter does not.
func TestFindSmartCase(t *testing.T) {
	t.Parallel()
	m := newFoldModel(t)
	for _, tt := range []struct {
		q    string
		want int
	}{
		{"LINE", 2},    // the new lines 10 and 30
		{"line 1", 12}, // lines 1 and 11 to 19, and both sides of 10
		{"Line", 0},
	} {
		if got := len(m.findHits(tt.q)); got != tt.want {
			t.Errorf("%d hits of %q, want %d", got, tt.q, tt.want)
		}
	}
}

// TestFindOpensFold checks that a step to a hit in a fold opens the fold.
func TestFindOpensFold(t *testing.T) {
	t.Parallel()
	m := newFoldModel(t)
	m = toTop(m) // the fold of lines 1 to 6
	m = find(t, m, "line 2")
	m = press(t, m, "enter")
	if r := curRow(m); r.New != 2 {
		t.Errorf("enter: the cursor is on %+v, want line 2", r)
	}
	if f := m.changes.rows[0].Fold; f.Lines > 0 {
		t.Errorf("the fold %+v stays closed, want it open", f)
	}
	if l := lastLine(m); !strings.HasSuffix(l, "1/11") {
		t.Errorf("footer %q, want 1/11", l)
	}
}

// TestFindSkipsLineNumbers checks that a find matches the text of the
// lines, and not their numbers.
func TestFindSkipsLineNumbers(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t)
	if m = find(t, m, "5"); !strings.HasSuffix(lastLine(m), "no matches") {
		t.Errorf("a find of a line number: footer %q, want no matches", lastLine(m))
	}
	m = press(t, m, "enter")
	if m.changes.cur != 0 {
		t.Errorf("enter with no hit: cursor %d, want 0", m.changes.cur)
	}
}

// TestFindSteps checks that enter and down go to the next hit, up goes
// to the previous hit, and both wrap around. Esc drops the query.
func TestFindSteps(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t)
	m = toTop(m)
	m = find(t, m, "return")
	if l := lastLine(m); !strings.HasPrefix(l, "find return") || !strings.HasSuffix(l, " 2") {
		t.Errorf("off a hit: footer %q, want the query and 2", l)
	}
	wantOld := func(m model, old int, count string) {
		t.Helper()
		if r := curRow(m); (old > 0 && r.Old != old) || (old == 0 && r.New != 5) {
			t.Errorf("the cursor is on %+v, want old line %d or new line 5", r, old)
		}
		if l := lastLine(m); !strings.HasSuffix(l, count) {
			t.Errorf("footer %q, want %s", l, count)
		}
	}
	m = press(t, m, "enter")
	wantOld(m, 4, "1/2")
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	wantOld(m, 0, "2/2")
	m = press(t, m, "enter")
	wantOld(m, 4, "1/2")
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	wantOld(m, 0, "2/2")

	if m = press(t, m, "esc"); m.mode != modeNormal {
		t.Fatalf("esc: mode %v, want normal", m.mode)
	}
	if m = find(t, m, ""); m.find.input.Value() != "" {
		t.Errorf("ctrl+f after esc: query %q, want none", m.find.input.Value())
	}
}

// TestFindKeepsCommentKeys checks that n types into the band, and that n
// goes to the next comment after the band closes.
func TestFindKeepsCommentKeys(t *testing.T) {
	t.Parallel()
	m := commented(t)
	line := curRow(m).New
	m = toTop(m)
	if m = find(t, m, "n"); m.find.input.Value() != "n" || m.mode != modeFind {
		t.Fatalf("n in the band: query %q, mode %v; want n typed", m.find.input.Value(), m.mode)
	}
	m = press(t, m, "esc")
	m = toTop(m)
	if m = press(t, m, "n"); curRow(m).New != line {
		t.Errorf("n: the cursor is on %+v, want the comment on line %d", curRow(m), line)
	}
}

// TestFindNotOnPR checks that the PR tab has no find band.
func TestFindNotOnPR(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t)
	m = press(t, m, "3")
	if m = step(t, m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl}); m.mode != modeNormal {
		t.Errorf("ctrl+f on the PR tab: mode %v, want normal", m.mode)
	}
}
