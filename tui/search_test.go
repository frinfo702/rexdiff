package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/rednafi/rexdiff/internal/search"
	"github.com/rednafi/rexdiff/internal/testgit"
	"github.com/rednafi/rexdiff/vcs"
)

// typeQuery types q in the search and returns the command of the last key.
func typeQuery(t *testing.T, m model, q string) (model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, r := range q {
		next, c := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m, cmd = *next.(*model), c
	}
	return m, cmd
}

// openQuery opens the search and types q. It runs the code search and
// gives its result to the model.
func openQuery(t *testing.T, m model, q string) model {
	t.Helper()
	m = press(t, m, "/")
	m, cmd := typeQuery(t, m, q)
	if cmd == nil {
		return m
	}
	return step(t, m, findGrep(cmd()))
}

// findGrep returns the grepMsg in msg, which can be a batch.
func findGrep(msg tea.Msg) tea.Msg {
	if b, ok := msg.(tea.BatchMsg); ok {
		for _, c := range b {
			if c == nil {
				continue
			}
			if g, ok := c().(grepMsg); ok {
				return g
			}
		}
	}
	return msg
}

// labels returns the labels of the rows of the search.
func labels(m model) []string {
	var ls []string
	for _, i := range m.overlay.shown {
		ls = append(ls, m.overlay.items[i].label)
	}
	return ls
}

var tabKey = tea.KeyPressMsg{Code: tea.KeyTab}

// TestSearchFlip checks that tab switches the mode of the search and
// keeps the query, as in herdr-reviewr.
func TestSearchFlip(t *testing.T) {
	m, _ := newTestModel(t)
	m = openQuery(t, m, "package")
	if m.overlay.search.code {
		t.Fatal("the search opens in the code mode, want files")
	}
	if !shows(m, "files 0 │ code 2") {
		t.Errorf("view got:\n%s\nwant the chips \"files 0 │ code 2\"", view(m))
	}
	m = step(t, m, tabKey)
	if !m.overlay.search.code || m.overlay.input.Value() != "package" {
		t.Errorf("after tab: code %t, query %q; want true, %q",
			m.overlay.search.code, m.overlay.input.Value(), "package")
	}
	if want := []string{"a.go", "    1: package a", "b.go", "    1: package a"}; !slices.Equal(labels(m), want) {
		t.Errorf("the code rows are %q, want %q", labels(m), want)
	}
	m = step(t, m, tabKey)
	if m.overlay.search.code {
		t.Error("a second tab stays in the code mode, want files")
	}
}

// TestSearchFileLimit checks that the search shows 50 files, then a
// "… more" row, and the count of all the matches in the chip.
func TestSearchFileLimit(t *testing.T) {
	m, dir := newTestModel(t)
	for i := range 60 {
		testgit.WriteFile(t, dir, fmt.Sprintf("zz%02d.txt", i), "x\n")
	}
	m = openQuery(t, m, "zz")
	ls := labels(m)
	if len(ls) != 51 || ls[50] != "… more" {
		t.Errorf("the search shows %d rows, the last %q; want 51, \"… more\"", len(ls), ls[len(ls)-1])
	}
	if !shows(m, "files 60 │") {
		t.Errorf("view got:\n%s\nwant \"files 60 │\"", view(m))
	}
	// The cursor stops on the last file, not on the "… more" row.
	for range 60 {
		m = step(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.overlay.cur != 49 {
		t.Errorf("after 60 downs the cursor is on row %d, want 49", m.overlay.cur)
	}
}

// TestSearchCodeGroups checks that the code matches show under a header
// for each file, and that the cursor skips the headers.
func TestSearchCodeGroups(t *testing.T) {
	m, _ := newTestModel(t)
	m = openQuery(t, m, "package")
	m = step(t, m, tabKey)
	if it := m.overlay.selected(); it == nil || it.id != "a.go" || it.line != 1 {
		t.Fatalf("the cursor is on %+v, want the match in a.go", it)
	}
	m = step(t, m, tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	if it := m.overlay.selected(); it == nil || it.id != "b.go" || m.overlay.cur != 3 {
		t.Errorf("after ctrl+n the cursor is on row %d, %+v; want 3, b.go", m.overlay.cur, it)
	}
	m = step(t, m, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if m.overlay.cur != 1 {
		t.Errorf("after ctrl+p the cursor is on row %d, want 1", m.overlay.cur)
	}
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.overlay.cur != 1 {
		t.Errorf("up from the first match moves the cursor to row %d, want 1", m.overlay.cur)
	}
}

// TestSearchCodeMore checks the "+" of the chip and the "… more" row when
// git finds more matches than the search shows.
func TestSearchCodeMore(t *testing.T) {
	m, dir := newTestModel(t)
	testgit.WriteFile(t, dir, "many.txt", strings.Repeat("hit\n", 300))
	m = openQuery(t, m, "hit")
	m = step(t, m, tabKey)
	ls := labels(m)
	if len(ls) != 202 || ls[201] != "… more" {
		t.Errorf("the search shows %d rows; want 202, the last \"… more\"", len(ls))
	}
	if !shows(m, "code 200+") {
		t.Errorf("view got:\n%s\nwant \"code 200+\"", view(m))
	}
}

// TestSearchStaleGrep checks that a code search for an old query shows
// nothing, and that a change of the query stops the old search.
func TestSearchStaleGrep(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "/")
	m, first := typeQuery(t, m, "pack")
	m, _ = typeQuery(t, m, "z")
	msg := findGrep(first())
	if g, ok := msg.(grepMsg); !ok || g.err == nil {
		t.Errorf("the old code search returns %+v, want a canceled search", msg)
	}
	m = step(t, m, grepMsg{query: "pack", hits: []vcs.Match{{Path: "a.go", Line: 1, Text: "old"}}})
	if len(m.overlay.search.hits) != 0 {
		t.Errorf("an old result shows %+v, want nothing", m.overlay.search.hits)
	}
}

// TestSearchEmptyQuery checks that an empty query lists the files that
// the user picks in this session first, then the other files, and no
// code.
func TestSearchEmptyQuery(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "/")
	if want := []string{"a.go", "b.go"}; !slices.Equal(labels(m), want) {
		t.Errorf("an empty query lists %q, want %q", labels(m), want)
	}
	m, _ = typeQuery(t, m, "b.go")
	m = press(t, m, "enter", "/")
	if want := []string{"b.go", "a.go"}; !slices.Equal(labels(m), want) {
		t.Errorf("after a pick of b.go, an empty query lists %q, want %q", labels(m), want)
	}
	m = step(t, m, tabKey)
	if len(labels(m)) != 0 || shows(m, "no matches") {
		t.Errorf("the code mode of an empty query shows %q, want nothing", labels(m))
	}
}

// TestSearchNoMatches checks the message of a search with no result.
func TestSearchNoMatches(t *testing.T) {
	m, _ := newTestModel(t)
	m = openQuery(t, m, "qqqq")
	if !shows(m, "no matches") {
		t.Errorf("view got:\n%s\nwant \"no matches\"", view(m))
	}
	m = step(t, m, tabKey)
	if !shows(m, "no matches") {
		t.Errorf("code mode view got:\n%s\nwant \"no matches\"", view(m))
	}
}

// longFile writes a file of 100 lines, with "needle" on line 62.
func longFile(t *testing.T, dir string) {
	t.Helper()
	var b strings.Builder
	for i := range 100 {
		if i == 61 {
			b.WriteString("needle here\n")
			continue
		}
		fmt.Fprintf(&b, "line %d\n", i+1)
	}
	testgit.WriteFile(t, dir, "long.go", b.String())
}

// lineOf returns the index of the first line of the view that has s, or
// -1.
func lineOf(m model, s string) int {
	return slices.IndexFunc(strings.Split(view(m), "\n"), func(l string) bool {
		return strings.Contains(l, s)
	})
}

// TestSearchPreview checks that the preview shows the file of the code
// match below the results, with the match in the middle, and that the
// results and the preview share the space, as in herdr-reviewr.
func TestSearchPreview(t *testing.T) {
	m, dir := newTestModel(t)
	longFile(t, dir)
	m = openQuery(t, m, "needle")
	m = step(t, m, tabKey)
	results, rule := lineOf(m, "─ results ─"), lineOf(m, "─ preview · long.go ─")
	hit, end := lineOf(m, "▶   62  needle here"), lineOf(m, "╰")
	if results < 0 || rule < 0 || hit < 0 {
		t.Fatalf("view got:\n%s\nwant the results, the preview of long.go, and line 62 marked", view(m))
	}
	if top, bottom := rule-results, end-rule; top-bottom > 1 || bottom-top > 1 {
		t.Errorf("the results take %d lines and the preview %d, want the same", top, bottom)
	}
	if mid := (rule + end) / 2; hit < mid-1 || hit > mid+1 {
		t.Errorf("the match is on line %d of the view, want the middle of the preview, %d", hit, mid)
	}
}

// TestSearchPreviewScroll checks that pgdown and pgup scroll the preview.
func TestSearchPreviewScroll(t *testing.T) {
	m, dir := newTestModel(t)
	longFile(t, dir)
	m = openQuery(t, m, "needle")
	m = step(t, m, tabKey)
	_ = view(m)
	top := m.overlay.search.preview.top
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	_ = view(m)
	if got := m.overlay.search.preview.top; got != top+pickerPage {
		t.Errorf("after pgdown the preview starts at %d, want %d", got, top+pickerPage)
	}
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	_ = view(m)
	if got := m.overlay.search.preview.top; got != top-pickerPage {
		t.Errorf("after 2 pgups the preview starts at %d, want %d", got, top-pickerPage)
	}
	if m.overlay.input.Value() != "needle" {
		t.Errorf("the query is %q after the scroll, want \"needle\"", m.overlay.input.Value())
	}
}

// TestSearchPreviewFile checks the preview of a file in the files mode,
// and the note of a binary file and of no result.
func TestSearchPreviewFile(t *testing.T) {
	m, dir := newTestModel(t)
	testgit.WriteFile(t, dir, "bin.dat", "a\x00b")
	m = openQuery(t, m, "a.go")
	if !shows(m, "─ preview · a.go ─", "1  package a") {
		t.Errorf("view got:\n%s\nwant the preview of a.go from line 1", view(m))
	}
	m = press(t, m, "esc")
	m = openQuery(t, m, "bin.dat")
	if !shows(m, noteBinary) {
		t.Errorf("view got:\n%s\nwant %q", view(m), noteBinary)
	}
	m = press(t, m, "esc")
	m = openQuery(t, m, "qqqq")
	if !shows(m, "─ preview ─", "no preview") {
		t.Errorf("view got:\n%s\nwant \"no preview\"", view(m))
	}
}

// TestSearchBudget checks that a code search past its time budget shows
// the matches that it finds by then, with a "+" in the chip.
// TestGrepDeadline in vcs checks the matches.
func TestSearchBudget(t *testing.T) {
	m, _ := newTestModel(t)
	m.grepBudget = search.CodeBudget
	m = press(t, m, "/")
	bin := t.TempDir()
	// The fake git prints one match, then hangs.
	testgit.Script(t, bin, "git", "printf 'a.go\\0001\\000hit\\n'\nexec /bin/sleep 10\n")
	t.Setenv("PATH", bin)
	m.overlay.input.SetValue("hit")
	cmd := m.inputChanged()
	start := time.Now()
	msg := cmd()
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("the code search takes %v, want about %v", d, search.CodeBudget)
	}
	// On a slow machine, the budget can end before the match.
	if m = step(t, m, msg); !shows(m, "code 1+") && !shows(m, "code 0+") {
		t.Errorf("view got:\n%s\nwant the chip \"code 1+\"", view(m))
	}
}
