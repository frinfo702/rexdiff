package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/rednafi/rexdiff/diff"
	"github.com/rednafi/rexdiff/rex"
	"github.com/rednafi/rexdiff/vcs"
)

// newTestModel makes a repository and a model that shows it. The branch
// main has a.go. The branch feat adds b.go. The working tree changes a.go.
func newTestModel(t testing.TB) (model, string) {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) { git(t, dir, args...) }
	run("init", "-q", "-b", "main")
	write(t, dir, "a.go", "package a\n\nfunc A() int {\n\treturn 1\n}\n")
	run("add", ".")
	run("commit", "-qm", "init")
	run("checkout", "-qb", "feat")
	write(t, dir, "b.go", "package a\n")
	run("add", ".")
	run("commit", "-qm", "add b")
	write(t, dir, "a.go", "package a\n\nfunc A() int {\n\tx := 2\n\treturn x\n}\n")

	repo, err := vcs.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(t.Context(), Options{Repo: repo}, defaultConfig())
	m.reload()
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	return m, dir
}

// git runs git in dir and fails the test if git fails.
func git(t testing.TB, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"GIT_AUTHOR_NAME=t",
		"GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@t",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
}

// write writes a file in dir and fails the test if it cannot.
func write(t testing.TB, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// update gives msg to m and returns the new model.
func update(t testing.TB, m model, msg tea.Msg) model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(model)
}

// press sends keys to m. A key is a name, such as "enter", or the text
// that the key types, such as "j".
func press(t *testing.T, m model, keys ...string) model {
	t.Helper()
	named := map[string]rune{
		"enter": tea.KeyEnter,
		"esc":   tea.KeyEscape,
		"home":  tea.KeyHome,
		"end":   tea.KeyEnd,
	}
	for _, k := range keys {
		code, ok := named[k]
		switch {
		case ok:
			m = update(t, m, tea.KeyPressMsg{Code: code})
		case len(k) > 1 && strings.Trim(k, "abcdefghijklmnopqrstuvwxyz") == "":
			t.Fatalf("press: no key is named %q", k)
		default:
			m = update(t, m, tea.KeyPressMsg{Code: []rune(k)[0], Text: k})
		}
	}
	return m
}

// view returns the screen of m without escape sequences.
func view(m model) string { return ansi.Strip(m.View().Content) }

// shows reports whether the screen of m has each of texts.
func shows(m model, texts ...string) bool {
	v := view(m)
	for _, s := range texts {
		if !strings.Contains(v, s) {
			return false
		}
	}
	return true
}

// status returns the event that the agent app in block is in state.
func status(block, app string, state rex.State) eventMsg {
	return eventMsg{Block: block, Status: rex.Status{App: app, State: state}}
}

// paths returns the paths of the files in the Changes tab.
func paths(m model) []string {
	var ps []string
	for _, f := range m.files {
		ps = append(ps, f.Path)
	}
	return ps
}

func TestScopes(t *testing.T) {
	m, dir := newTestModel(t)
	if got := paths(m); !slices.Equal(got, []string{"a.go"}) {
		t.Fatalf("uncommitted scope shows %v, want [a.go]", got)
	}
	if f := m.files[0]; f.adds != 2 || f.dels != 1 {
		t.Errorf("a.go: +%d -%d, want +2 -1", f.adds, f.dels)
	}

	m = press(t, m, "b")
	if got := paths(m); !slices.Equal(got, []string{"a.go", "b.go"}) {
		t.Errorf("branch scope vs %s shows %v, want [a.go b.go]", m.base, got)
	}

	// The commit picker lists the newest commit first; enter picks it.
	m = press(t, m, "G", "enter")
	if got := paths(m); m.scope != scopeCommits || !slices.Equal(got, []string{"b.go"}) {
		t.Errorf("commits scope shows %v, want [b.go]", got)
	}
	// A click on the commit in the header opens the commit picker again.
	top, _, _ := strings.Cut(view(m), "\n")
	i := strings.Index(top, shortSHA(m.to))
	m = update(t, m, tea.MouseClickMsg{Button: tea.MouseLeft, X: ansi.StringWidth(top[:i])})
	if m.mode != modeOverlay || m.overlay.kind != overlayCommits {
		t.Errorf("a click on the commit in the header did not open the commit picker")
	}
	m = press(t, m, "esc")

	// A turn of an agent: the scope shows only what changed during it.
	m = press(t, m, "t")
	if !strings.Contains(m.changes.note, "no agent turn") {
		t.Errorf("turn scope before a turn: note %q", m.changes.note)
	}
	m = update(t, m, status("block:agent", "pi", rex.StateWorking))
	write(t, dir, "c.go", "package a\n")
	m = update(t, m, status("block:agent", "pi", rex.StateIdle))
	if got := paths(m); !slices.Equal(got, []string{"c.go"}) || m.turn.running {
		t.Errorf("turn scope shows %v (running %v), want [c.go]", got, m.turn.running)
	}
	if v := view(m); !strings.Contains(v, "pi · ") {
		t.Errorf("header does not name the agent of the turn:\n%s", v)
	}
}

func TestComment(t *testing.T) {
	m, _ := newTestModel(t)

	// Unified: three rows for the change.
	m = press(t, m, "]", "v", "j", "j", "c")
	if m.mode != modeComment {
		t.Fatal("c did not open the comment box")
	}
	m = press(t, m, "h", "i", "enter")
	// Split: the same change takes two rows.
	m = press(t, m, "|", "home", "]", "v", "j", "c", "o", "k", "enter")

	cs := m.comments.All()
	if len(cs) != 2 {
		t.Fatalf("got %d comments, want 2", len(cs))
	}
	rows := "a.go:4-5\n-\treturn 1\n+\tx := 2\n+\treturn x\n"
	if got, want := m.comments.Review(""), rows+"hi\n\n"+rows+"ok"; got != want {
		t.Errorf("Review:\n%s\nwant:\n%s", got, want)
	}
	if !shows(m, "💬 hi", "2 unsent comments") {
		t.Errorf("the view does not show the comments:\n%s", view(m))
	}

	m = press(t, m, "s")
	if !strings.Contains(m.status, "not inside Rex") {
		t.Errorf("send outside Rex: status %q", m.status)
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if m = next.(model); cmd == nil || m.status != "copied 2 comments" {
		t.Errorf("copy: status %q, command %v", m.status, cmd)
	}

	// Quit asks first, because the comments are not sent.
	if m = press(t, m, "q"); m.overlay == nil || m.overlay.kind != overlayQuit {
		t.Error("q with unsent comments did not ask to confirm")
	}
}

func TestTabs(t *testing.T) {
	m, _ := newTestModel(t)

	m = press(t, m, "2")
	if !shows(m, "Files (2)", "package a") {
		t.Errorf("the Files tab does not list 2 files and show a.go:\n%s", view(m))
	}

	// The repository has no remote, so the PR tab shows why.
	m = press(t, m, "3")
	m = update(t, m, m.loadPR()())
	if v := view(m); !strings.Contains(v, "no git remote") {
		t.Errorf("the PR tab does not say that there is no git remote:\n%s", v)
	}
}

// TestPRChecks shows the checks of a pull request: the counts, and the
// names of the checks that failed.
func TestPRChecks(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "3")
	pr := &vcs.PullRequest{
		Title: "T",
		State: vcs.StateOpen,
		Checks: []vcs.Check{
			{Name: "test", Result: vcs.ResultPass},
			{Name: "docs", Result: vcs.ResultSkip},
			{Name: "build", Result: vcs.ResultFail},
			{Name: "e2e", Result: vcs.ResultPending},
		},
	}
	m = update(t, m, prMsg{pr: pr})
	if !shows(m, "checks: ✓ 2  ✗ 1  ◌ 1", "✗ build") {
		t.Errorf("the PR tab does not show the checks:\n%s", view(m))
	}
}

// TestSmallScreens presses every key on screens down to 1x1. The program
// must not panic, and the view must fit the screen.
func TestSmallScreens(t *testing.T) {
	keys := []string{
		"w", "j", "v", "j", "c", "\x1b[31mx\x07", "enter", "f", "F", "]", "[", "n", "N",
		"|", "p", "m", "t", "b", "u", "G", "esc", "/", "a", "esc", ":", "2", "enter", "?",
		"esc", "2", "j", "m", "3", "1", "x", "d", "y", "s", "esc", "q", "esc", "home",
	}
	sizes := []struct{ w, h int }{
		{0, 0}, {1, 1}, {2, 2}, {10, 3}, {20, 5}, {40, 4}, {200, 6},
	}
	for _, size := range sizes {
		m, _ := newTestModel(t)
		m = update(t, m, tea.WindowSizeMsg{Width: size.w, Height: size.h})
		for _, k := range keys {
			m = press(t, m, k)
			lines := strings.Split(view(m), "\n")
			if len(lines) > max(size.h, 1) {
				t.Errorf("%dx%d after %q: %d lines", size.w, size.h, k, len(lines))
			}
			for _, l := range lines {
				if w := ansi.StringWidth(l); w > max(size.w, 1) {
					t.Errorf("%dx%d after %q: line width %d: %q", size.w, size.h, k, w, l)
					break
				}
			}
		}
	}
}

// TestDragAfterReload drags while the pane gets shorter. The program must
// not panic.
func TestDragAfterReload(t *testing.T) {
	m, _ := newTestModel(t)
	m.drag = dragState{active: true, gutter: true, moved: true, start: 48}
	m = update(t, m, tea.MouseReleaseMsg{Button: tea.MouseLeft})

	m.changes.set(nil, nil) // a reload found no changes
	m.drag = dragState{active: true}
	m = update(t, m, tea.MouseMotionMsg{Button: tea.MouseLeft, Y: m.h})
	view(m)
}

// screenY returns the screen line of row i of the current pane.
func screenY(t *testing.T, m model, i int) int {
	t.Helper()
	_, rowAt := m.mainLines(m.bodyHeight())
	y := slices.Index(rowAt, i)
	if y < 0 {
		t.Fatalf("row %d is not on the screen", i)
	}
	return bodyY + y
}

// click presses and releases the left button at x, y.
func click(t *testing.T, m model, x, y int) model {
	t.Helper()
	m = update(t, m, tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	return update(t, m, tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x, Y: y})
}

// TestClickOldNumber clicks the old line number of an unchanged line. The
// comment goes on the new side, so that it shows and d can delete it.
func TestClickOldNumber(t *testing.T) {
	m, _ := newTestModel(t)
	equal := func(r diff.Row) bool { return r.Kind == diff.KindEqual }
	i := slices.IndexFunc(m.changes.rows, equal)
	m = click(t, m, m.mainX()+gutter, screenY(t, m, i))
	m = press(t, m, "h", "i", "enter")
	if v := view(m); !strings.Contains(v, "💬 hi") {
		t.Errorf("the comment does not show:\n%s", v)
	}
	if m = press(t, m, "d"); m.comments.Len() != 0 {
		t.Errorf("d did not delete the comment: %+v", m.comments.All())
	}
}

// TestClickCommentBox clicks while the comment box is open. As in
// herdr-reviewr, no click changes the draft or closes the box.
func TestClickCommentBox(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "]", "c", "h", "i")
	_, rowAt := m.mainLines(m.bodyHeight())
	box := bodyY + slices.Index(rowAt, m.changes.cur) + 1
	clicks := []struct {
		where string
		y     int
	}{
		{"on", box},
		{"outside", screenY(t, m, 0)},
	}
	for _, c := range clicks {
		m = click(t, m, m.mainX()+6, c.y)
		if m.mode != modeComment || m.editor.Value() != "hi" {
			t.Errorf("a click %s the box: mode %v, draft %q; want the box open with hi",
				c.where, m.mode, m.editor.Value())
		}
	}
}

// errorHas reports whether err is not nil and has each of texts.
func errorHas(err error, texts ...string) bool {
	if err == nil {
		return false
	}
	for _, s := range texts {
		if !strings.Contains(err.Error(), s) {
			return false
		}
	}
	return true
}

// wrapModel shows a change that makes line 4 of a.go too long for the
// screen, which is 80 columns wide. Wrap is off.
func wrapModel(t *testing.T) (m model, dir, long string) {
	t.Helper()
	m, dir = newTestModel(t)
	long = "\treturn " + strings.Repeat("x + ", 40) + "1"
	write(t, dir, "a.go", "package a\n\nfunc A() int {\n"+long+"\n}\n")
	m.reload()
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 40})
	return m, dir, long
}

// twoHunks commits 20 equal lines below line 4 of a.go. Then it changes
// line 4 and the line below the 20 lines, so the change has two hunks.
func twoHunks(t *testing.T, m model, dir, long string) model {
	t.Helper()
	same := strings.Repeat("\tx++\n", 20)
	write(t, dir, "a.go", "package a\n\nfunc A() int {\n\treturn 1\n"+same+"}\n")
	git(t, dir, "commit", "-qam", "same")
	write(t, dir, "a.go", "package a\n\nfunc A() int {\n"+long+"\n"+same+"\treturn 9\n}\n")
	m.reload()
	return m
}

// hunks returns the rows of the hunk headers in the Changes tab.
func hunks(m model) []int {
	var rs []int
	for i, r := range m.changes.rows {
		if r.Kind == diff.KindHunk {
			rs = append(rs, i)
		}
	}
	return rs
}

// TestMouseOffScreen sends mouse events outside the screen, also while an
// overlay is open and the screen changes size. The program must not panic.
func TestMouseOffScreen(t *testing.T) {
	m, _ := newTestModel(t)
	for _, keys := range [][]string{{}, {"G"}, {"/"}, {"2"}, {"3"}} {
		m = press(t, m, keys...)
		points := []struct{ x, y int }{
			{-1, -1}, {-50, 3}, {3, -50}, {1 << 30, 1 << 30}, {m.w, m.h},
		}
		for _, pt := range points {
			mouse := tea.Mouse{X: pt.x, Y: pt.y, Button: tea.MouseLeft}
			m = update(t, m, tea.MouseClickMsg(mouse))
			m = update(t, m, tea.MouseMotionMsg(mouse))
			m = update(t, m, tea.MouseReleaseMsg(mouse))
			mouse.Button = tea.MouseWheelDown
			m = update(t, m, tea.MouseWheelMsg(mouse))
		}
		m = update(t, m, tea.WindowSizeMsg{Width: 5, Height: 3})
		m = press(t, m, "w", "w", "w")
		m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
		view(m)
		m = press(t, m, "esc", "1")
	}
}
