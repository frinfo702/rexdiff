package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// commented returns a model with the comment "hi" on the first change.
func commented(t *testing.T) model {
	t.Helper()
	m, _ := newTestModel(t)
	m = press(t, m, "]", "c", "h", "i", "enter")
	if m.comments.Len() != 1 {
		t.Fatalf("got %d comments, want 1", m.comments.Len())
	}
	return m
}

// TestCommentAddsNew checks that c on a line with a comment adds a second
// comment, as in herdr-reviewr.
func TestCommentAddsNew(t *testing.T) {
	m := press(t, commented(t), "c", "o", "k", "enter")
	if m.comments.Len() != 2 || m.status != "comment added" {
		t.Errorf("c on a comment: %d comments, status %q; want 2, \"comment added\"",
			m.comments.Len(), m.status)
	}
}

// TestEditBlankKeeps checks that an edit with blank text cancels the edit
// and keeps the comment, as in herdr-reviewr.
func TestEditBlankKeeps(t *testing.T) {
	m := press(t, commented(t), "e")
	if m.mode != modeComment || m.editor.Value() != "hi" {
		t.Fatalf("e on a comment: mode %v, text %q; want the box with hi",
			m.mode, m.editor.Value())
	}
	m.status = ""
	m.editor.SetValue("  ")
	m = press(t, m, "enter")
	cs := m.comments.All()
	if len(cs) != 1 || cs[0].Text != "hi" || m.status != "" || m.mode != modeNormal {
		t.Errorf("a blank edit: %d comments, status %q, mode %v; want hi kept, no status",
			len(cs), m.status, m.mode)
	}
}

// TestDeleteNeedsDiffFocus checks that c and d act only with the focus on
// the diff.
func TestDeleteNeedsDiffFocus(t *testing.T) {
	m := commented(t)
	tab := tea.KeyPressMsg{Code: tea.KeyTab}
	if m = step(t, m, tab); !m.focusList {
		t.Fatal("tab: the focus stays on the diff, want the file list")
	}
	if m = press(t, m, "d"); m.comments.Len() != 1 {
		t.Errorf("d with the list focus: %d comments, want 1", m.comments.Len())
	}
	if m = press(t, m, "c"); m.mode != modeNormal {
		t.Errorf("c with the list focus: mode %v, want no comment box", m.mode)
	}
	m = press(t, step(t, m, tab), "d")
	if m.comments.Len() != 0 || m.status != "comment deleted" {
		t.Errorf("d on the diff: %d comments, status %q; want 0, \"comment deleted\"",
			m.comments.Len(), m.status)
	}
}

// TestCommentList checks the keys of the comment list.
func TestCommentList(t *testing.T) {
	m := press(t, commented(t), "c", "o", "k", "enter", "l")
	if !shows(m, "Comments (2)", "s send 2 · esc close · y copy · e edit · d delete") {
		t.Errorf("the list got:\n%s\nwant the title and the keys", view(m))
	}
	if m = press(t, m, "j"); m.overlay.cur != 1 {
		t.Errorf("j in the list: row %d, want 1", m.overlay.cur)
	}
	if m = press(t, m, "k"); m.overlay.cur != 0 {
		t.Errorf("k in the list: row %d, want 0", m.overlay.cur)
	}
	for _, k := range []string{"esc", "l"} {
		if m := press(t, m, k); m.overlay != nil {
			t.Errorf("%s in the list: the list stays open, want it closed", k)
		}
	}
	if m := press(t, m, "s"); m.overlay == nil || !strings.Contains(m.status, "not inside Rex") {
		t.Errorf("s in the list: overlay %v, status %q; want a send", m.overlay, m.status)
	}

	// e edits the comment, and the list opens again after the save.
	m = press(t, m, "j", "e")
	if m.mode != modeComment || m.editor.Value() != "ok" {
		t.Fatalf("e in the list: mode %v, text %q; want the box with ok",
			m.mode, m.editor.Value())
	}
	m = press(t, m, "!", "enter")
	if m.overlay == nil || m.overlay.kind != overlayComments || m.overlay.cur != 1 {
		t.Fatalf("after the edit: overlay %v, want the list at row 1", m.overlay)
	}
	if got := m.comments.All()[1].Text; got != "ok!" || m.status != "comment updated" {
		t.Errorf("after the edit: text %q, status %q; want ok!, \"comment updated\"",
			got, m.status)
	}

	// d deletes, and the list closes with the last comment.
	if m = press(t, m, "d"); m.comments.Len() != 1 || !shows(m, "Comments (1)") {
		t.Errorf("d in the list: %d comments, view:\n%s\nwant 1", m.comments.Len(), view(m))
	}
	if m = press(t, m, "d"); m.overlay != nil || m.comments.Len() != 0 {
		t.Errorf("d on the last comment: overlay %v, want the list closed", m.overlay)
	}
}

// TestCommentListCopy checks that y in the comment list copies, clears the
// comments, and closes the list.
func TestCommentListCopy(t *testing.T) {
	fakeClipboard(t, false)
	m := press(t, commented(t), "l")
	m = stepRun(t, m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	if m.overlay != nil || m.comments.Len() != 0 || m.status != "copied 1 comment" {
		t.Errorf("y in the list: overlay %v, %d comments, status %q; want a copy",
			m.overlay, m.comments.Len(), m.status)
	}
}
