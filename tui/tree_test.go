package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rednafi/rexdiff/internal/testgit"
	"github.com/rednafi/rexdiff/vcs"
)

// treeRepo is the repository of newTreeModel. It has changes in src/a.go,
// src/b.go, and top.go, and an ignored directory out.
var treeRepo = testgit.NewFixture(func(tb testing.TB, dir string) {
	tb.Helper()
	run := func(args ...string) { testgit.Git(tb, dir, args...) }
	testgit.NewRepo(tb, dir)
	for _, p := range []string{"src/a.go", "src/b.go", "top.go"} {
		testgit.WriteFile(tb, dir, p, "package a\n")
	}
	testgit.WriteFile(tb, dir, ".gitignore", "out/\n")
	run("add", ".")
	run("commit", "-qm", "init")
	for _, p := range []string{"src/a.go", "src/b.go", "top.go"} {
		testgit.WriteFile(tb, dir, p, "package a\n\nvar x = 1\n")
	}
	testgit.WriteFile(tb, dir, "out/x.txt", "x\n")
	testgit.WriteFile(tb, dir, "out/sub/y.txt", "y\n")
})

// newTreeModel copies treeRepo to a new directory. The model shows the
// Changes tab with the focus on the file list.
func newTreeModel(t *testing.T) (model, string) {
	t.Helper()
	dir := t.TempDir()
	treeRepo.Copy(t, dir)
	repo, err := vcs.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(t.Context(), Options{Repo: repo}, defaultConfig())
	setClock(&m)
	m.reload()
	m = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m.focusList = true
	return m, dir
}

// rowNames returns the names of the rows of the file list of the current
// tab, with "/" after a directory.
func rowNames(m model) []string {
	var out []string
	for _, r := range m.list().rows {
		n := r.Name
		if r.File < 0 {
			n += "/"
		}
		out = append(out, n)
	}
	return out
}

// shownPath returns the path of the file that the current tab shows.
func shownPath(m model) string {
	if m.tab == tabFiles {
		return m.allFiles[m.allSel].Path
	}
	return m.changedFile().Path
}

// TestTreeKeys checks the cursor, the expand and collapse keys, and the
// keys f and F in the tree of the Changes tab.
func TestTreeKeys(t *testing.T) {
	t.Parallel()
	m, _ := newTreeModel(t)
	if got, want := rowNames(m), []string{"src/", "a.go", "b.go", "top.go"}; !slices.Equal(got, want) {
		t.Fatalf("rows %q, want %q", got, want)
	}
	if m.changeTree.cur != 1 || shownPath(m) != "src/a.go" {
		t.Fatalf("start: cursor row %d on %s, want row 1 on src/a.go", m.changeTree.cur, shownPath(m))
	}

	// On a directory, the pane keeps the file.
	m = press(t, m, "k")
	if m.changeTree.cur != 0 || shownPath(m) != "src/a.go" {
		t.Errorf("k: cursor row %d, shows %s; want row 0, src/a.go", m.changeTree.cur, shownPath(m))
	}
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if got, want := rowNames(m), []string{"src/", "top.go"}; !slices.Equal(got, want) {
		t.Errorf("← on src: rows %q, want %q", got, want)
	}
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	if got := rowNames(m); len(got) != 4 {
		t.Errorf("→ on src: rows %q, want src expanded", got)
	}

	// f and F skip the directory.
	m = press(t, m, "f")
	if shownPath(m) != "src/a.go" || m.changeTree.cur != 1 {
		t.Errorf("f from src: shows %s at row %d, want src/a.go at row 1", shownPath(m), m.changeTree.cur)
	}
	m = press(t, m, "f", "f")
	if shownPath(m) != "top.go" {
		t.Errorf("f f: shows %s, want top.go", shownPath(m))
	}
	m = press(t, m, "F", "F", "F")
	if shownPath(m) != "src/a.go" || m.changeTree.cur != 1 {
		t.Errorf("F past the first file: shows %s at row %d, want src/a.go at row 1",
			shownPath(m), m.changeTree.cur)
	}

	// A file row keeps its +N -N, and a collapsed directory keeps the
	// cursor after a reload.
	if !shows(m, "a.go", "+2 -0") {
		t.Errorf("view got:\n%s\nwant a.go with +2 -0", view(m))
	}
	m = press(t, m, "k")
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyLeft})
	m.reload()
	if r, ok := m.changeTree.dir(); !ok || r.Dir != "src" || r.Expanded {
		t.Errorf("after a reload: cursor on %+v, want on the collapsed src", r)
	}
}

// TestFileTree checks the tree of the Files tab: the directories start
// collapsed, a collapsed directory with a change has a dot, a click
// toggles, and an ignored directory reads its entries when it expands.
func TestFileTree(t *testing.T) {
	t.Parallel()
	m, _ := newTreeModel(t)
	m = press(t, m, "2")
	m.focusList = true
	if got, want := rowNames(m), []string{"out/", "src/", ".gitignore", "top.go"}; !slices.Equal(got, want) {
		t.Fatalf("rows %q, want %q", got, want)
	}
	if got := listLine(m, "src/"); !strings.Contains(got, "•") {
		t.Errorf("collapsed src is %q, want a dot for its change", got)
	}
	if got := listLine(m, "out/"); strings.Contains(got, "•") {
		t.Errorf("out is %q, want no dot", got)
	}

	// A click on src expands it, and a second click collapses it.
	y := m.listY() + 1 + slices.Index(rowNames(m), "src/")
	m = click(t, m, m.listX()+3, y)
	if got := rowNames(m); !slices.Contains(got, "a.go") {
		t.Errorf("after a click on src: rows %q, want its files", got)
	}
	m.click = clickChain{} // not a double click
	m = click(t, m, m.listX()+3, y)
	if got := rowNames(m); slices.Contains(got, "a.go") {
		t.Errorf("after a second click on src: rows %q, want it collapsed", got)
	}

	// → on the ignored out reads its entries.
	m.fileTree.cur = 0
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	want := []string{"out/", "sub/", "x.txt", "src/", ".gitignore", "top.go"}
	if got := rowNames(m); !slices.Equal(got, want) {
		t.Fatalf("→ on out: rows %q, want %q", got, want)
	}
	if r := m.fileTree.rows[2]; !r.Ignored || m.allFiles[r.File].Path != "out/x.txt" {
		t.Errorf("x.txt row %+v, want the ignored out/x.txt", r)
	}
	// The expanded directory stays open after a reload.
	m.reloadFiles()
	if got := rowNames(m); !slices.Equal(got, want) {
		t.Errorf("after a reload: rows %q, want %q", got, want)
	}
	m = press(t, m, "j", "j")
	if shownPath(m) != "out/x.txt" || !shows(m, "1  x") {
		t.Errorf("j j: shows %s, view:\n%s\nwant out/x.txt", shownPath(m), view(m))
	}
}

// listLine returns the line of the file list that has text.
func listLine(m model, text string) string {
	for _, l := range m.listLines(m.listRows()) {
		if l := ansi.Strip(l); strings.Contains(l, text) {
			return l
		}
	}
	return ""
}

// TestShownIgnored checks that the watcher gets the expanded ignored
// directories of the Files tab, as in herdr-reviewr.
func TestShownIgnored(t *testing.T) {
	t.Parallel()
	m, _ := newTreeModel(t)
	m = press(t, m, "2")
	if got := m.shownIgnored(); len(got) != 0 {
		t.Errorf("collapsed: %q, want none", got)
	}
	m.fileTree.cur = 0
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyRight}) // expand out
	if got := m.shownIgnored(); !slices.Equal(got, []string{"out"}) {
		t.Errorf("out expanded: %q, want [out]", got)
	}
	if m = press(t, m, "1"); len(m.shownIgnored()) != 0 {
		t.Errorf("in the Changes tab: %q, want none", m.shownIgnored())
	}
}
