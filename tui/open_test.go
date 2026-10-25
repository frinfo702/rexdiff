package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/rednafi/rexdiff/internal/testgit"
	"github.com/rednafi/rexdiff/vcs"
)

// prURL is the URL of the pull request in prModel. The opener must get it
// as one argument.
const prURL = "https://example.com/o/r/pull/1?a=1&b='x y'"

// prModel returns a test model on the PR tab with a pull request.
func prModel(t *testing.T) model {
	t.Helper()
	m, _ := newTestModel(t)
	m = press(t, m, "3")
	return step(t, m, prMsg{gen: m.prf.gen, pr: &vcs.PullRequest{Title: "T", URL: prURL}})
}

// fifo makes a named pipe in a new directory and returns its path. The
// opener writes its arguments to it, so the test can wait for the opener
// without a sleep.
func fifo(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "args")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// readFifo returns what a writer writes to the pipe at path. It fails the
// test if no writer comes.
func readFifo(t *testing.T, path string) string {
	t.Helper()
	got := make(chan string, 1)
	go func() {
		b, _ := os.ReadFile(path) // blocks until the writer closes the pipe
		got <- string(b)
	}()
	select {
	case s := <-got:
		return s
	case <-time.After(10 * time.Second): // a guard; the opener runs at once
		t.Fatal("the opener does not run")
		return ""
	}
}

// TestOpenURLSetting checks that o opens the pull request with url_opener.
func TestOpenURLSetting(t *testing.T) {
	t.Parallel()
	m := prModel(t)
	out := fifo(t)
	m.cfg.URLOpener = `printf '%s\n' {url} > ` + out
	m = press(t, m, "o")
	if got := readFifo(t, out); got != prURL+"\n" {
		t.Errorf("the opener got %q, want %q", got, prURL+"\n")
	}
	if m.status != "" {
		t.Errorf("status %q, want none", m.status)
	}
}

// TestOpenURLDefault checks that o opens the pull request with the opener
// of the system, and shows why the opener does not start.
func TestOpenURLDefault(t *testing.T) {
	m := prModel(t)
	bin := t.TempDir()
	out := fifo(t)
	for _, name := range []string{"open", "xdg-open"} {
		testgit.Script(t, bin, name, `printf '%s\n' "${0##*/}" "$@" > '`+out+"'\n")
	}
	t.Setenv("PATH", bin)
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	m = press(t, m, "o")
	if got, want := readFifo(t, out), opener+"\n"+prURL+"\n"; got != want {
		t.Errorf("the opener got %q, want %q", got, want)
	}

	t.Setenv("PATH", t.TempDir())
	m = press(t, m, "o")
	if want := `open: exec: "` + opener + `": executable file not found in $PATH`; m.status != want {
		t.Errorf("status %q, want %q", m.status, want)
	}
}
