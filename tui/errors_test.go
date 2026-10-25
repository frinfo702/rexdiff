package tui

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/rednafi/rexdiff/rex"
)

// noGit makes git not found for the rest of the test.
func noGit(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
}

// TestPickerErrors checks that a picker shows the error of git and does
// not open.
func TestPickerErrors(t *testing.T) {
	for _, k := range []string{"B", "G"} {
		t.Run(k, func(t *testing.T) {
			m, _ := newTestModel(t)
			noGit(t)
			m = press(t, m, k)
			if m.overlay != nil {
				t.Errorf("%s opens an overlay, want none", k)
			}
			if !shows(m, "git isn't installed") {
				t.Errorf("view got:\n%s\nwant \"git isn't installed\"", view(m))
			}
		})
	}
}

// TestFilesRefreshError checks that the Files tab shows the error of
// git.
func TestFilesRefreshError(t *testing.T) {
	m, _ := newTestModel(t)
	noGit(t)
	if m = press(t, m, "2"); !shows(m, "refresh failed: ", "git isn't installed") {
		t.Errorf("view got:\n%s\nwant \"refresh failed: …git isn't installed\"", view(m))
	}
}

// TestFilesRefreshKeepsList checks that a failed refresh keeps the list of
// files.
func TestFilesRefreshKeepsList(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "2")
	n := len(m.allFiles)
	noGit(t)
	m.reloadFiles()
	if len(m.allFiles) != n || n == 0 {
		t.Errorf("after a failed refresh, %d files, want %d", len(m.allFiles), n)
	}
}

// TestSearchError checks that the search shows the error of the code
// search in place of the results.
func TestSearchError(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "/")
	noGit(t)
	m.overlay.input.SetValue("pac")
	cmd := m.inputChanged()
	if cmd == nil {
		t.Fatal("typing in the search returns no command, want a grep")
	}
	m = step(t, m, cmd())
	if !shows(m, "git isn't installed") {
		t.Errorf("view got:\n%s\nwant \"git isn't installed\"", view(m))
	}
	if len(m.overlay.shown) != 0 {
		t.Errorf("the search shows %d results under the error, want 0", len(m.overlay.shown))
	}
}

// TestTurnSnapshotErrorLogs checks that a failed snapshot at the start of
// a turn goes to the log.
func TestTurnSnapshotErrorLogs(t *testing.T) {
	m, _ := newTestModel(t)
	var buf bytes.Buffer
	m.log = slog.New(slog.NewTextHandler(&buf, nil))
	noGit(t)
	m = step(t, m, status("block:agent", "pi", rex.StateWorking))
	if m.turn != nil {
		t.Errorf("turn = %+v, want none", m.turn)
	}
	if !strings.Contains(buf.String(), "turn snapshot failed") {
		t.Errorf("log got:\n%s\nwant \"turn snapshot failed\"", buf.String())
	}
}
