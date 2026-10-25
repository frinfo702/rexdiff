package tui

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/rednafi/rexdiff/internal/testgit"
)

// watched returns a test model with a watcher, after its first tick. The
// clock of the test does not move, so each tick checks the watcher.
func watched(t *testing.T) (m model, dir string) {
	t.Helper()
	m, dir = newTestModel(t)
	if err := m.startWatch(); err != nil {
		t.Fatal(err)
	}
	return step(t, m, tickMsg{}), dir
}

// TestTickRunsNoGitWhenIdle checks that a tick runs no git process while
// nothing changes. A git on PATH logs each call.
func TestTickRunsNoGitWhenIdle(t *testing.T) {
	m, dir := watched(t)
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "git.log")
	testgit.Script(t, bin, "git", `echo "$*" >> '`+log+"'\nexec '"+gitPath+"' \"$@\"\n")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for range 3 {
		m = step(t, m, tickMsg{})
	}
	if b, err := os.ReadFile(log); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("idle ticks run git:\n%s(%v)\nwant no call", b, err)
	}
	testgit.WriteFile(t, dir, "a.go", "package a\n\nfunc A() int {\n\treturn 42\n}\n")
	if m = step(t, m, tickMsg{}); !shows(m, "return 42") {
		t.Errorf("view got:\n%s\nwant the edit", view(m))
	}
	if _, err := os.Stat(log); err != nil {
		t.Errorf("a tick after an edit runs no git through the log: %v", err)
	}
}

// TestTickSeesEdit checks that a tick shows an edit.
func TestTickSeesEdit(t *testing.T) {
	t.Parallel()
	m, dir := watched(t)
	testgit.WriteFile(t, dir, "a.go", "package a\n\nfunc A() int {\n\treturn 42\n}\n")
	if m = step(t, m, tickMsg{}); !shows(m, "return 42") {
		t.Errorf("view got:\n%s\nwant the edit", view(m))
	}
}

// TestRefreshForces checks that r reloads in the pause after a reload.
func TestRefreshForces(t *testing.T) {
	t.Parallel()
	m, dir := watched(t)
	m.reloadAt = m.now().Add(time.Hour) // the pause after a slow reload
	testgit.WriteFile(t, dir, "a.go", "package a\n\nfunc A() int {\n\treturn 42\n}\n")
	if m = press(t, m, "r"); !shows(m, "return 42") {
		t.Errorf("view got:\n%s\nwant the edit", view(m))
	}
}

// TestTickRetries checks that a tick retries a failed reload after the
// wait, and that the wait then clears.
func TestTickRetries(t *testing.T) {
	m, _ := watched(t)
	clk := setClock(&m)
	path := os.Getenv("PATH")
	noGit(t)
	if m = press(t, m, "r"); m.retryAt.IsZero() || !strings.HasPrefix(m.status, "refresh failed: ") {
		t.Fatalf("after a failed reload: retry at %v, status %q; want a wait, a flash",
			m.retryAt, m.status)
	}
	t.Setenv("PATH", path)
	retryAt := m.retryAt
	if m = step(t, m, tickMsg{}); m.retryAt != retryAt {
		t.Errorf("a tick in the wait: retry at %v, want %v", m.retryAt, retryAt)
	}
	clk.advance(retryFirst)
	m = step(t, m, tickMsg{})
	if !m.retryAt.IsZero() || m.retryWait != 0 {
		t.Errorf("after a good reload, retryAt %v, retryWait %v; want zero", m.retryAt, m.retryWait)
	}
}

// TestRetryBackoff checks that each failure doubles the wait, from 1 s to
// 60 s.
func TestRetryBackoff(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t)
	want := []int{1, 2, 4, 8, 16, 32, 60, 60}
	for i, w := range want {
		before := m.now()
		m.retry(errors.New("fail"))
		got := m.retryAt.Sub(before).Round(time.Second)
		if got != time.Duration(w)*time.Second {
			t.Errorf("failure %d: wait %v, want %ds", i+1, got, w)
		}
	}
}

// watchedTree returns newTreeModel on the Files tab, with a watcher, after
// its first tick.
func watchedTree(t *testing.T) (m model, dir string) {
	t.Helper()
	m, dir = newTreeModel(t)
	if err := m.startWatch(); err != nil {
		t.Fatal(err)
	}
	m = press(t, m, "2")
	return step(t, m, tickMsg{}), dir
}

// openFile moves the cursor of the file list down to the file at path.
func openFile(t *testing.T, m model, path string) model {
	t.Helper()
	for range len(m.fileTree.rows) {
		if shownPath(m) == path {
			return m
		}
		m = press(t, m, "j")
	}
	if shownPath(m) != path {
		t.Fatalf("shows %s, want %s", shownPath(m), path)
	}
	return m
}

// TestFilesTabWaitsForWatcher checks that, with a watcher, a tick reads
// the open file of the Files tab only after the watcher reports a change.
func TestFilesTabWaitsForWatcher(t *testing.T) {
	t.Parallel()
	m, dir := watchedTree(t)
	const path = "top.go"
	m = openFile(t, m, path)
	fi, err := os.Stat(filepath.Join(dir, path))
	if err != nil {
		t.Fatal(err)
	}
	// An edit that keeps the size and the time is not a change for the
	// watcher.
	testgit.WriteFile(t, dir, path, "package a\n\nvar x = 2\n")
	if err := os.Chtimes(filepath.Join(dir, path), fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	if m = step(t, m, tickMsg{}); shows(m, "var x = 2") {
		t.Errorf("no change for the watcher, view got:\n%s\nwant no read", view(m))
	}
	testgit.WriteFile(t, dir, path, "package a\n\nvar x = 42\n")
	if m = step(t, m, tickMsg{}); !shows(m, "var x = 42") {
		t.Errorf("view got:\n%s\nwant the edit", view(m))
	}
}

// TestTickSeesIgnoredEdit checks that a tick shows an edit of the open
// file in an expanded ignored directory, as in herdr-reviewr.
func TestTickSeesIgnoredEdit(t *testing.T) {
	t.Parallel()
	m, dir := watchedTree(t)
	m.fileTree.cur = 0
	m = step(t, m, tea.KeyPressMsg{Code: tea.KeyRight}) // expand out
	m = openFile(t, m, "out/x.txt")
	m = step(t, m, tickMsg{}) // the watcher gets the open file
	testgit.WriteFile(t, dir, "out/x.txt", "an edit\n")
	if m = step(t, m, tickMsg{}); !shows(m, "an edit") {
		t.Errorf("view got:\n%s\nwant the edit", view(m))
	}
}
