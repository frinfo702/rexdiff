package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// fakeClipboard puts a pbcopy first on PATH. It writes its input to the
// file that fakeClipboard returns. With fail, it exits 1.
func fakeClipboard(t *testing.T, fail bool) string {
	t.Helper()
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	script := "#!/bin/sh\n/bin/cat > " + out + "\n"
	if fail {
		script = "#!/bin/sh\nexit 1\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "pbcopy"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return out
}

// stepRun gives msg to m, then gives m the message of the command that
// m returns.
func stepRun(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	next, cmd := m.Update(msg)
	m, _ = run(t, *next.(*model), cmd)
	return m
}

// TestCopyClearsComments checks that y copies the review and clears the
// comments, as a send does.
func TestCopyClearsComments(t *testing.T) {
	out := fakeClipboard(t, false)
	m, _ := newTestModel(t)
	m = press(t, m, "]", "c", "h", "i", "enter")
	want := m.reviewText()
	m = stepRun(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	if m.status != "copied 1 comment" || m.comments.Len() != 0 {
		t.Errorf("after y: status %q, %d comments; want %q, 0",
			m.status, m.comments.Len(), "copied 1 comment")
	}
	got, err := os.ReadFile(out)
	if err != nil || string(got) != want {
		t.Errorf("the clipboard got %q (%v), want %q", got, err, want)
	}
}

// TestCopyFails checks that a failed copy keeps the comments and shows
// why.
func TestCopyFails(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T)
		want  string
	}{
		{"no tool", func(t *testing.T) { t.Setenv("PATH", t.TempDir()) },
			"copy failed: install wl-clipboard, xclip, or xsel"},
		{"exit 1", func(t *testing.T) { fakeClipboard(t, true) }, "copy failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestModel(t)
			m = press(t, m, "]", "c", "h", "i", "enter")
			tt.setup(t)
			m = stepRun(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"})
			if m.status != tt.want || m.comments.Len() != 1 || m.sends != 0 {
				t.Errorf("after y: status %q, %d comments, %d sends; want %q, 1, 0",
					m.status, m.comments.Len(), m.sends, tt.want)
			}
		})
	}
}

// TestCopyFromList checks that a copy closes the comment list when it
// clears the last comment.
func TestCopyFromList(t *testing.T) {
	fakeClipboard(t, false)
	m, _ := newTestModel(t)
	m = press(t, m, "]", "c", "h", "i", "enter", "l")
	if m.overlay == nil || m.overlay.kind != overlayComments {
		t.Fatal("after l: no comment list, want the list")
	}
	m.onCopied(copiedMsg{ids: []int{m.comments.All()[0].ID}, done: "copied 1 comment"})
	if m.overlay != nil || m.mode != modeNormal {
		t.Errorf("after the copy: overlay %v, want the list closed", m.overlay)
	}
}

// TestDragCopies checks that a drag on the code copies the lines through
// the clipboard tool.
func TestDragCopies(t *testing.T) {
	out := fakeClipboard(t, false)
	m, _ := newTestModel(t)
	x := m.mainX() + m.w/2
	y0 := screenY(t, m, 0)
	m = step(t, m, tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y0})
	m = step(t, m, tea.MouseMotionMsg{Button: tea.MouseLeft, X: x, Y: y0 + 1})
	m = stepRun(t, m, tea.MouseReleaseMsg{Button: tea.MouseLeft, X: x, Y: y0 + 1})
	got, err := os.ReadFile(out)
	if err != nil || !strings.HasPrefix(m.status, "copied 2 lines") ||
		strings.Count(string(got), "\n") != 1 {
		t.Errorf("after a drag: status %q, clipboard %q (%v); want 2 lines",
			m.status, got, err)
	}
}
