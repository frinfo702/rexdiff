package tui

import (
	"errors"
	"os"
	"testing"
	"time"
)

// watched returns a test model with a watcher, after its first tick. The
// next check of the watcher is due after the time that it returns.
func watched(t *testing.T) (m model, dir string, pause time.Duration) {
	t.Helper()
	m, dir = newTestModel(t)
	w, err := m.repo.Watch(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	m.watch = w
	start := time.Now()
	m = step(t, m, tickMsg{})
	// The pause of the watcher is 20 times its check, which is a part of
	// the tick. A check after a change can take longer than the first
	// check, thus wait twice as long.
	return m, dir, 42 * time.Since(start)
}

// TestTickRunsNoGitWhenIdle checks that a tick runs no git process while
// nothing changes.
func TestTickRunsNoGitWhenIdle(t *testing.T) {
	m, _, pause := watched(t)
	noGit(t)
	for range 3 {
		time.Sleep(pause)
		m = step(t, m, tickMsg{})
	}
	if shows(m, "isn't installed") {
		t.Errorf("view got:\n%s\nwant no git error", view(m))
	}
}

// TestTickSeesEdit checks that a tick shows an edit.
func TestTickSeesEdit(t *testing.T) {
	m, dir, pause := watched(t)
	write(t, dir, "a.go", "package a\n\nfunc A() int {\n\treturn 42\n}\n")
	time.Sleep(pause)
	if m = step(t, m, tickMsg{}); !shows(m, "return 42") {
		t.Errorf("view got:\n%s\nwant the edit", view(m))
	}
}

// TestRefreshForces checks that r reloads in the pause after a reload.
func TestRefreshForces(t *testing.T) {
	m, dir, _ := watched(t)
	m.reloadAt = time.Now().Add(time.Hour)
	write(t, dir, "a.go", "package a\n\nfunc A() int {\n\treturn 42\n}\n")
	if m = press(t, m, "r"); !shows(m, "return 42") {
		t.Errorf("view got:\n%s\nwant the edit", view(m))
	}
}

// TestTickRetries checks that a tick retries a failed reload after the
// wait, and that the error then clears.
func TestTickRetries(t *testing.T) {
	m, _, _ := watched(t)
	path := os.Getenv("PATH")
	noGit(t)
	if m = press(t, m, "r"); !shows(m, "unknown revision") {
		t.Fatalf("view got:\n%s\nwant the git error", view(m))
	}
	t.Setenv("PATH", path)
	if m = step(t, m, tickMsg{}); !shows(m, "unknown revision") {
		t.Errorf("view got:\n%s\nwant the error until the wait ends", view(m))
	}
	m.retryAt, m.reloadAt = time.Now(), time.Now() // the waits end
	if m = step(t, m, tickMsg{}); shows(m, "unknown revision") {
		t.Errorf("view got:\n%s\nwant no error after the retry", view(m))
	}
	if !m.retryAt.IsZero() || m.retryWait != 0 {
		t.Errorf("after a good reload, retryAt %v, retryWait %v; want zero", m.retryAt, m.retryWait)
	}
}

// TestRetryBackoff checks that each failure doubles the wait, from 1 s to
// 60 s.
func TestRetryBackoff(t *testing.T) {
	m, _ := newTestModel(t)
	want := []int{1, 2, 4, 8, 16, 32, 60, 60}
	for i, w := range want {
		before := time.Now()
		m.retry(errors.New("fail"))
		got := m.retryAt.Sub(before).Round(time.Second)
		if got != time.Duration(w)*time.Second {
			t.Errorf("failure %d: wait %v, want %ds", i+1, got, w)
		}
	}
}
