package tui

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rednafi/rexdiff/internal/testgit"
	"github.com/rednafi/rexdiff/vcs"
)

// TestNoReloadInCommentBox checks that a tick does not reload while the
// comment box is open, and that the reload runs when the box closes.
func TestNoReloadInCommentBox(t *testing.T) {
	t.Parallel()
	m, dir := newTestModel(t)
	m = press(t, m, "]", "c")
	testgit.WriteFile(t, dir, "a.go", "package a\n\nfunc A() int {\n\treturn 42\n}\n")
	if m = step(t, m, tickMsg{}); shows(m, "return 42") || !m.reloadOwed {
		t.Errorf("a tick with the box open: owed %v, view:\n%s\nwant no reload, a reload owed",
			m.reloadOwed, view(m))
	}
	if m = press(t, m, "esc"); !shows(m, "return 42") {
		t.Errorf("after the box closes, view got:\n%s\nwant the edit", view(m))
	}
}

// TestReloadErrorKeepsFiles checks that a failed reload keeps the files,
// the selection, and the cursor, and flashes the error.
func TestReloadErrorKeepsFiles(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "]", "j")
	cur, sel, n := m.changes.cur, m.changeSel, len(m.files)
	noGit(t)
	m = press(t, m, "r")
	if len(m.files) != n || m.changeSel != sel || m.changes.cur != cur || m.shownTrees == "" {
		t.Errorf("after a failed reload: %d files, sel %d, cursor %d, trees %q; want %d, %d, %d, kept",
			len(m.files), m.changeSel, m.changes.cur, m.shownTrees, n, sel, cur)
	}
	if !strings.HasPrefix(m.status, "refresh failed: ") || m.changes.note != "" {
		t.Errorf("status %q, note %q; want \"refresh failed: …\", no note", m.status, m.changes.note)
	}
}

// TestReloadErrorNote checks that a failed load shows the error in place
// of the files when no files show.
func TestReloadErrorNote(t *testing.T) {
	m, dir := newTestModel(t)
	testgit.Git(t, dir, "commit", "-qam", "all")
	if m = press(t, m, "r"); len(m.files) != 0 {
		t.Fatalf("after a commit of all changes: %d files, want none", len(m.files))
	}
	noGit(t)
	if m = press(t, m, "r"); m.changes.note == "" || m.changes.note == "no changes" {
		t.Errorf("a failed reload over no changes: note %q, want the error", m.changes.note)
	}
	m = press(t, m, "b") // another scope has nothing loaded
	if m.files != nil || m.changes.note == "" {
		t.Errorf("a failed first load: %d files, note %q; want none, the error", len(m.files), m.changes.note)
	}
}

// TestPRErrorKeepsPR checks that a failed read keeps the last good pull
// request, and that the end of the pull request clears it.
func TestPRErrorKeepsPR(t *testing.T) {
	t.Parallel()
	m, _ := newTestModel(t)
	m = press(t, m, "3")
	m = step(t, m, prMsg{gen: m.prf.gen, pr: &vcs.PullRequest{Title: "Good"}})
	m = step(t, m, prMsg{gen: m.prf.gen, err: errors.New("network down")})
	if m.pr.pr == nil || !shows(m, "Good") || !strings.Contains(m.status, "network down") {
		t.Errorf("after a failed read: pr %+v, status %q, view:\n%s\nwant the PR kept, a flash",
			m.pr.pr, m.status, view(m))
	}
	m = step(t, m, prMsg{gen: m.prf.gen, err: vcs.ErrNoPullRequest})
	if m.pr.pr != nil {
		t.Errorf("after no pull request: pr %+v, want none", m.pr.pr)
	}
}

// TestPollWithoutWatcher checks that without a watcher a tick reloads
// only every pollEvery.
func TestPollWithoutWatcher(t *testing.T) {
	t.Parallel()
	m, dir := newTestModel(t)
	clk := setClock(&m)
	m = step(t, m, tickMsg{})
	testgit.WriteFile(t, dir, "a.go", "package a\n\nfunc A() int {\n\treturn 42\n}\n")
	if m = step(t, m, tickMsg{}); shows(m, "return 42") {
		t.Errorf("a tick before the poll is due, view got:\n%s\nwant no reload", view(m))
	}
	clk.advance(pollEvery - time.Nanosecond)
	if m = step(t, m, tickMsg{}); shows(m, "return 42") {
		t.Errorf("a tick just before pollEvery, view got:\n%s\nwant no reload", view(m))
	}
	clk.advance(time.Nanosecond)
	if m = step(t, m, tickMsg{}); !shows(m, "return 42") {
		t.Errorf("a tick after pollEvery, view got:\n%s\nwant the edit", view(m))
	}
}

// TestWatchRestarts checks that a tick starts the watcher again after a
// failure, with a wait that doubles.
func TestWatchRestarts(t *testing.T) {
	m, _ := newTestModel(t)
	clk := setClock(&m)
	m.dropWatch(errors.New("gone"))
	clk.advance(retryFirst)
	path := os.Getenv("PATH")
	noGit(t)
	if m = step(t, m, tickMsg{}); m.watch != nil || !m.watchRetry.at.After(m.now()) {
		t.Fatalf("a failed start: watcher %v, next try %v; want none, a wait", m.watch, m.watchRetry.at)
	}
	t.Setenv("PATH", path)
	if m = step(t, m, tickMsg{}); m.watch != nil {
		t.Fatal("a tick before the wait ends starts the watcher")
	}
	clk.advance(2 * retryFirst)
	if m = step(t, m, tickMsg{}); m.watch == nil {
		t.Error("a tick after the wait: no watcher, want one")
	}
}

// TestWatchErrorRestarts checks that an error of the watcher drops it, so
// that a later tick starts it again.
func TestWatchErrorRestarts(t *testing.T) {
	m, dir := watched(t)
	noGit(t)
	testgit.WriteFile(t, dir, "new.go", "package a\n") // the watcher lists the files again
	if m = step(t, m, tickMsg{}); m.watch != nil || m.watchRetry.at.IsZero() {
		t.Errorf("after an error of the watcher: watcher %v, next try %v; want none, a try",
			m.watch, m.watchRetry.at)
	}
}
