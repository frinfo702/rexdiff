package tui

import (
	"errors"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
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

// TestEditorError opens an editor that cannot start. The footer tells
// why.
func TestEditorError(t *testing.T) {
	m, _ := newTestModel(t)
	m.cfg.Editor = "ed {file}"
	t.Setenv("PATH", "") // sh cannot start
	if m = press(t, m, "e"); !strings.Contains(view(m), "editor: ") {
		t.Errorf("the footer does not show the error:\n%s", view(m))
	}
}

// TestSendError sends the review when Rex fails. The footer shows the
// error of Rex.
func TestSendError(t *testing.T) {
	m, dir := newTestModel(t)
	bin := filepath.Join(dir, "rex")
	script := "#!/bin/sh\necho 'Error: no Rex server' >&2\nexit 1\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REX_SESSION", "S")
	t.Setenv("REX_BIN_DIR", dir)
	m.rex = rex.FromEnv()
	m = press(t, m, "]", "c", "h", "i", "enter")
	m = update(t, m, m.deliver(rex.Agent{Block: "b", Name: "pi"})())
	if !strings.Contains(m.status, "no Rex server") {
		t.Errorf("status %q, want the error of Rex", m.status)
	}
}

// TestMissingHead removes the object of the HEAD commit. The Changes tab
// shows the error, and not each file as added.
func TestMissingHead(t *testing.T) {
	m, dir := newTestModel(t)
	sha, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	hex := strings.TrimSpace(string(sha))
	obj := filepath.Join(dir, ".git", "objects", hex[:2], hex[2:])
	if err := os.Remove(obj); err != nil {
		t.Fatal(err)
	}
	m.reload()
	if v := view(m); len(m.files) > 0 || !strings.Contains(v, "unknown revision HEAD") {
		t.Errorf("files %v, want none and the error:\n%s", paths(m), v)
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

// TestLoadConfig checks the configuration file: a herdr-reviewr file
// carries over, and an unknown key or bad value is an error.
func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	load := func(content string) error {
		t.Helper()
		write(t, filepath.Dir(path), "config.toml", content)
		_, err := loadConfig(path)
		return err
	}

	// The actions in herdr-reviewr's src/keymap.rs.
	names := strings.Fields(`base-pick collapse comment comments commit-pick copy
		delete down edit expand find goto-line half-down half-up keys navigator-grow
		navigator-hide navigator-position navigator-shrink next-comment next-file
		next-hunk open-pr page-down page-up prev-comment prev-file prev-hunk quit
		quit-discard refresh rendered scope-branch scope-commits scope-last-turn
		scope-uncommitted search select send split tab-all-files tab-changes tab-pr up
		wrap`)
	herdr := "theme = \"terminal\"\n[keybindings]\n"
	for _, n := range names {
		herdr += n + " = [\"x\"]\n"
	}
	if err := load(herdr); err != nil {
		t.Errorf("herdr-reviewr config: %v", err)
	}

	unknown := []struct{ content, key string }{
		{"layuot = \"split\"\n", `"layuot"`},
		{"[keybindings]\nnext-fiel = [\"x\"]\n", `"keybindings.next-fiel"`},
		{"[extra]\nx = 1\n", `"extra"`},
	}
	for _, c := range unknown {
		if err := load(c.content); !errorHas(err, path, "unknown key "+c.key) {
			t.Errorf("config %q: error %v, want unknown key %s in %s",
				c.content, err, c.key, path)
		}
	}

	// As in herdr-reviewr, each bad value is an error that names the key,
	// the allowed values, and the value.
	const (
		editor = "editor: expected a command with only {file} and {line}"
		opener = "url_opener: expected a command with only {url}"
		scopes = "default_scope: expected one of uncommitted, branch, last-turn"
		theme  = "theme: expected terminal or a chroma style name"
		mdView = "markdown_view: expected one of source, rendered"
		host   = "gitlab_host: expected a bare hostname"
	)
	bad := []struct{ content, want string }{
		{`layout = "sideways"`, `layout: expected one of unified, split, not "sideways"`},
		{`default_scope = "commits"`, scopes + `, not "commits"`},
		{`markdown_view = "preview"`, mdView + `, not "preview"`},
		{`file_list = "top"`, `file_list: expected one of right, left, not "top"`},
		{`theme = "tokyo-night"`, theme + `, not "tokyo-night"`},
		{`file_list_width = 3`, `file_list_width: expected 16 or more, not "3"`},
		{`editor = " "`, editor + `, not " "`},
		{`editor = "code {fil"`, editor + `, not "code {fil"`},
		{`editor = "code {FILE}"`, `editor: expected`},
		{`url_opener = "open {ur}"`, opener + `, not "open {ur}"`},
		{`review_prefix = ""`, `review_prefix: expected text, not ""`},
		{`gitlab_host = "https://git.x"`, host + `, not "https://git.x"`},
		{"[keybindings]\nsend = []", `keybindings.send: expected a list of keys`},
		{`layout = 1`, `layout`},
	}
	for _, c := range bad {
		if err := load(c.content + "\n"); !errorHas(err, path, c.want) {
			t.Errorf("config %q: error %v, want %s in %s", c.content, err, c.want, path)
		}
	}

	good := `theme = "Monokai"
editor = "code -g {file}:{line}"
gitlab_host = "git.corp.example"
`
	if err := load(good); err != nil {
		t.Errorf("good config: %v", err)
	}
}

// TestConfigReload changes the configuration file while the program runs.
// A reload keeps the flags and the layout that the user picked.
func TestConfigReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	// The flag -layout=split wins over the file.
	write(t, filepath.Dir(path), "config.toml", "layout = \"unified\"\n")
	m, _ := newTestModel(t)
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	o := Options{Repo: m.repo, ConfigPath: path, Layout: LayoutSplit}
	m = newModel(t.Context(), o, cfg)
	if !m.split {
		t.Fatal("the flag -layout=split did not win over the file")
	}
	reload := func(content string) {
		t.Helper()
		write(t, filepath.Dir(path), "config.toml", content)
		m.cfgModTime = time.Time{} // the file changes in the same tick
		m = update(t, m, tickMsg{})
	}
	// A reload applies the file, keeps the flag, and does not undo |.
	m = press(t, m, "|")
	reload("layout = \"unified\"\nfile_list = \"left\"\n")
	if m.split || !m.listLeft || m.cfg.Layout != LayoutSplit || m.cfgErr != "" {
		t.Errorf("after a reload: split %v, list left %v, layout %q, error %q; "+
			"want false, true, split, none",
			m.split, m.listLeft, m.cfg.Layout, m.cfgErr)
	}
	// A bad file shows its error and keeps the last good configuration.
	bad := []struct{ content, want string }{
		{"file_list = \"right\"\nbogus = 1\n", `unknown key "bogus"`},
		{"file_list = \"center\"\n", `invalid value for file_list`},
	}
	for _, c := range bad {
		reload(c.content)
		if !strings.Contains(m.cfgErr, c.want) || !m.listLeft {
			t.Errorf("after a bad reload: error %q, list left %v; "+
				"want %s and the old config", m.cfgErr, m.listLeft, c.want)
		}
	}
	// A reload that does not change the file list does not undo p and >.
	m = press(t, m, "p", ">")
	reload("layout = \"unified\"\nfile_list = \"left\"\nreview_prefix = \"x\"\n")
	if m.listLeft || m.listW != 28 {
		t.Errorf("after a reload: list left %v, width %d; want false, 28",
			m.listLeft, m.listW)
	}
}

// TestPalette checks that inserted lines get their tint from the green of
// the terminal, as in pi's system theme.
func TestPalette(t *testing.T) {
	m, _ := newTestModel(t)
	m = update(t, m, tea.BackgroundColorMsg{Color: color.Black})
	// Black mixed 16% toward pure green.
	const tint = "48;2;0;40;0m"
	if strings.Contains(m.View().Content, tint) {
		t.Fatal("the view has the tint before the terminal reports its green")
	}
	m = update(t, m, uv.UnknownOscEvent("\x1b]4;2;rgb:0000/ffff/0000\x1b\\"))
	if !strings.Contains(m.View().Content, tint) {
		t.Error("inserted lines are not tinted toward the green of the terminal")
	}
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

// TestWrap checks wrap as in herdr-reviewr. The w key toggles it, and the
// cursor moves by source lines. A comment on wrapped lines gets the same
// snippet.
func TestWrap(t *testing.T) {
	m, _, long := wrapModel(t)
	cut := len(m.changes.lines)

	m = press(t, m, "w")
	if !m.wrap || len(m.changes.lines) <= cut {
		t.Fatalf("w: wrap %v, %d lines, want more than %d",
			m.wrap, len(m.changes.lines), cut)
	}
	if v := view(m); !strings.Contains(v, "x + 1") {
		t.Errorf("the end of the long line does not show:\n%s", v)
	}
	m = press(t, m, "]")
	for range len(m.changes.lines) {
		if m.changes.cont(m.changes.cur) {
			t.Fatalf("the cursor is on a continued row %d", m.changes.cur)
		}
		m = press(t, m, "j")
	}

	m = press(t, m, "home", "]", "v", "j", "c", "o", "k", "enter")
	cs := m.comments.All()
	want := "\n-\treturn 1\n+" + long + "\nok"
	if r := m.comments.Review(""); len(cs) != 1 || !strings.HasSuffix(r, want) {
		t.Fatalf("Review:\n%s\nwant one comment that ends with:%s", r, want)
	}
	// The comment shows below the last row of the wrapped line.
	lines := strings.Split(view(m), "\n")
	i := slices.IndexFunc(lines, func(l string) bool {
		return strings.Contains(l, "💬 ok")
	})
	if i < 1 || !strings.Contains(lines[i-1], "x + 1") {
		t.Errorf("the comment is not below the end of the line:\n%s", view(m))
	}

	// The sideways scroll applies only without wrap.
	if m = update(t, m, tea.KeyPressMsg{Code: tea.KeyRight}); m.changes.xoff != 0 {
		t.Errorf("→ with wrap scrolled to %d", m.changes.xoff)
	}
	if m = press(t, m, "w"); m.wrap || len(m.changes.lines) != cut {
		t.Errorf("w again: wrap %v, %d lines, want %d", m.wrap, len(m.changes.lines), cut)
	}
	// A w in the Files tab applies to the Changes tab too.
	if m = press(t, m, "2", "w", "w", "1"); len(m.changes.lines) != cut {
		t.Errorf("w in the Files tab: the Changes tab has %d lines, want %d",
			len(m.changes.lines), cut)
	}
}

// TestWrapKeepsCursor checks that w keeps the cursor on its source line or
// on its hunk header.
func TestWrapKeepsCursor(t *testing.T) {
	m, dir, long := wrapModel(t)
	m = press(t, m, "end", "w")
	if got := m.target(m.changes.cur, diff.SideNew).line; got != 5 {
		t.Errorf("w moved the cursor from line 5 to line %d", got)
	}

	m = twoHunks(t, m, dir, long)
	m.changes.cur = hunks(m)[1]
	if m = press(t, m, "w"); m.changes.cur != hunks(m)[1] {
		t.Errorf("w moved the cursor from the second hunk header to row %+v",
			m.changes.rows[m.changes.cur])
	}
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

// TestTallLine checks a wrapped line that is taller than the screen.
func TestTallLine(t *testing.T) {
	m, dir, long := wrapModel(t)
	m = press(t, m, "w")
	m = twoHunks(t, m, dir, long)
	m = update(t, m, tea.WindowSizeMsg{Width: 30, Height: 12})

	// The wheel scrolls past the line.
	m = press(t, m, "home")
	wheel := []tea.MouseButton{tea.MouseWheelDown, tea.MouseWheelDown, tea.MouseWheelUp}
	for _, b := range wheel {
		top := m.changes.top
		m = update(t, m, tea.MouseWheelMsg{Button: b, X: 1, Y: 5})
		m = update(t, m, tea.BlurMsg{}) // a later event keeps the view
		if m.changes.top == top {
			t.Fatalf("the wheel stopped at row %d of %d", top, len(m.changes.lines))
		}
	}
	// A taller screen shows more rows, not blank space below the view.
	m = update(t, m, tea.WindowSizeMsg{Width: 30, Height: 40})
	if last := max(len(m.changes.lines)-m.bodyHeight(), 0); m.changes.top > last {
		t.Errorf("top %d of %d rows on a screen of %d rows",
			m.changes.top, len(m.changes.lines), m.bodyHeight())
	}
	if m = press(t, m, "j"); m.changes.top > m.changes.cur {
		t.Errorf("j does not show the cursor: top %d, cursor %d",
			m.changes.top, m.changes.cur)
	}

	// As in herdr-reviewr, a cursor on the line shows its first rows. The
	// comment box shows below the last row. A c on a line with a comment
	// edits the comment.
	m = update(t, m, tea.WindowSizeMsg{Width: 30, Height: 12})
	m = press(t, m, "home")
	for m.changes.rows[m.changes.cur].New != 4 {
		m = press(t, m, "j")
	}
	if v := view(m); !strings.Contains(v, "return x") {
		t.Errorf("the first rows of the line do not show:\n%s", v)
	}
	m = press(t, m, "c", "o", "k", "enter", "c", "z", "z")
	lines := strings.Split(view(m), "\n")
	has := func(s string) func(string) bool {
		return func(l string) bool { return strings.Contains(l, s) }
	}
	i := slices.IndexFunc(lines, has("okzz"))
	if i < 0 || !slices.ContainsFunc(lines[:i], has("+ 1")) {
		t.Errorf("the comment box is not below the end of the line:\n%s", view(m))
	}
	if strings.Contains(view(m), "💬 ok") {
		t.Errorf("the comment shows above the box that edits it:\n%s", view(m))
	}
}

// TestHostileText puts escape sequences in outside data: a file name, a
// comment, an agent, and an error from Rex. The screen must
// not get them raw. The diff tests check the review text.
func TestHostileText(t *testing.T) {
	const osc = "\x1b]0;owned\x07" // sets the title of the terminal
	raw := func(where string, m model) {
		t.Helper()
		if strings.Contains(m.View().Content, osc) {
			t.Errorf("%s: the screen has the escape sequence raw", where)
		}
	}
	m, dir := newTestModel(t)
	write(t, dir, "e"+osc+"\n.go", "x"+osc+"\n")
	m.reload()
	raw("file list", m)
	m = press(t, m, "f", "]", "c")
	m = update(t, m, tea.PasteMsg{Content: "hi" + osc + "\x1b[201~\r"})
	m = press(t, m, "enter")
	raw("comment", m)
	if cs := m.comments.All(); len(cs) != 1 || !strings.HasPrefix(cs[0].Path, "e\x1b") {
		t.Fatalf("comments %+v, want one on the file with the escape sequence", cs)
	}

	m = update(t, m, sentMsg{agent: rex.Agent{Name: "pi" + osc}})
	raw("the status after a send", m)
	m = update(t, m, sentMsg{err: errors.New("Rex" + osc)})
	raw("the status after an error", m)
	m = update(t, m, status("b", "pi"+osc, rex.StateWorking))
	m = press(t, m, "t")
	raw("the turn in the header", m)
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

// TestEmptyRepo presses keys in each tab of a repository with no commits
// and no files, with key bindings that conflict. The program must not
// panic.
func TestEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	repo, err := vcs.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.Keybindings = keybindings{
		Down:    []string{},
		Comment: []string{"j", "c"},
		Send:    []string{"x"},
		Copy:    []string{"x"},
	}
	m := newModel(t.Context(), Options{Repo: repo}, cfg)
	m.reload()
	keys := []string{
		"j", "k", "v", "c", "x", "enter", "f", "]", "n", "|", "m", "t", "b", "g", "G",
		"enter", "B", "enter", "/", "a", "enter", ":", "9", "enter", "l", "esc", "e", "d",
		"y", "s", "o", "r", "z", "<", "home", "end", "w", "esc",
	}
	for _, tab := range []string{"1", "2", "3"} {
		m = press(t, m, tab)
		for _, k := range keys {
			m = press(t, m, k)
			view(m)
		}
	}
}

// TestFooterHints checks the footer in the select and the find modes. It
// shows the keys of the mode.
func TestFooterHints(t *testing.T) {
	m, _ := newTestModel(t)
	if m = press(t, m, "]", "v"); !strings.Contains(view(m), "extend") {
		t.Errorf("select mode: the footer has no hint:\n%s", view(m))
	}
	m = press(t, m, "esc")
	m = update(t, m, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	m = press(t, m, "r", "e", "t", "u", "r", "n", "enter")
	if !strings.Contains(view(m), "next/prev match") {
		t.Errorf("find mode: the footer has no hint:\n%s", view(m))
	}
}
