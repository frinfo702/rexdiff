package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rednafi/rexdiff/diff"
)

// TestCommentKeepsLine checks that a comment stays on its line numbers
// when lines come above it, as in herdr-reviewr.
func TestCommentKeepsLine(t *testing.T) {
	m := commented(t)
	c := m.comments.All()[0]
	start, end := c.Start, c.End
	write(t, m.repo.Root(), "a.go",
		"// one\n// two\npackage a\n\nfunc A() int {\n\tx := 2\n\treturn x\n}\n")
	m.reload()
	if c.Start != start || c.End != end {
		t.Errorf("after two lines above: lines %d-%d, want %d-%d", c.Start, c.End, start, end)
	}
	if m.comments.At(m.anchor(), "a.go", c.Side, start) != c {
		t.Errorf("no comment at line %d after the edit, want it", start)
	}
	if strings.Contains(view(m), "(stale)") {
		t.Errorf("the diff shows (stale):\n%s", view(m))
	}
}

// TestStaleInList checks that a comment whose file leaves the changes
// shows "(stale)" in the comment list only.
func TestStaleInList(t *testing.T) {
	m := commented(t)
	git(t, m.repo.Root(), "checkout", "--", "a.go")
	m.reload()
	if shows(m, "(stale)") {
		t.Errorf("the diff shows (stale):\n%s", view(m))
	}
	if m = press(t, m, "l"); !shows(m, "a.go:4", "(stale)") {
		t.Errorf("l after a revert got:\n%s\nwant the comment marked (stale)", view(m))
	}
}

// TestFileCommentView checks that a comment on the Files tab shows only
// there, and is stale only when its file is gone.
func TestFileCommentView(t *testing.T) {
	m, dir := newTestModel(t)
	m = press(t, m, "2", "j", "c", "f", "i", "l", "e", "enter")
	if m.comments.Len() != 1 || !shows(m, "💬 file") {
		t.Fatalf("a comment in Files: %d comments, view:\n%s", m.comments.Len(), view(m))
	}
	if c := m.comments.All()[0]; c.Anchor != (diff.Anchor{}) {
		t.Errorf("anchor = %+v, want a file view on the working tree", c.Anchor)
	}
	if m = press(t, m, "1"); shows(m, "💬 file") || shows(m, "●1") {
		t.Errorf("the Changes tab shows the Files comment:\n%s", view(m))
	}
	if m = press(t, m, "l"); shows(m, "(stale)") {
		t.Errorf("the list marks the comment (stale):\n%s", view(m))
	}
	m = press(t, m, "esc")
	if err := os.Remove(filepath.Join(dir, "a.go")); err != nil {
		t.Fatal(err)
	}
	if m = press(t, m, "l"); !shows(m, "(stale)") {
		t.Errorf("l after a delete got:\n%s\nwant (stale)", view(m))
	}
}

// TestCommitsCommentHidden checks that a comment on a commit pick shows
// only at that pick, and that the comment list takes no edit of it in
// another scope.
func TestCommitsCommentHidden(t *testing.T) {
	m, _ := newTestModel(t)
	m = press(t, m, "G", "enter") // the newest commit adds b.go
	if m.scope != scopeCommits {
		t.Fatalf("scope = %s, want commits", m.scope)
	}
	m = press(t, m, "]", "c", "p", "i", "c", "k", "enter")
	if m.comments.Len() != 1 || !shows(m, "💬 pick") {
		t.Fatalf("a comment on the pick: %d comments, view:\n%s", m.comments.Len(), view(m))
	}
	// The branch scope also changes b.go.
	m = press(t, m, "b")
	if i := m.fileIndex("b.go"); i < 0 {
		t.Fatalf("branch scope files %v, want b.go", paths(m))
	} else {
		m.pickFile(i)
	}
	if shows(m, "💬 pick") {
		t.Errorf("the branch scope shows the comment of the pick:\n%s", view(m))
	}
	m = press(t, m, "l")
	if shows(m, "e edit") {
		t.Errorf("the list offers an edit of another diff:\n%s", view(m))
	}
	if m = press(t, m, "e"); m.mode != modeOverlay {
		t.Errorf("e on a comment of another diff: mode %v, want the list open", m.mode)
	}
	m = press(t, m, "esc", "g")
	if i := m.fileIndex("b.go"); i >= 0 {
		m.pickFile(i)
	}
	if !shows(m, "💬 pick") {
		t.Errorf("back on the pick got:\n%s\nwant the comment", view(m))
	}
	if m = press(t, m, "l"); !shows(m, "e edit") {
		t.Errorf("the list at the pick got:\n%s\nwant e edit", view(m))
	}
}

// fileIndex returns the index of path in the Changes tab, or -1.
func (m *model) fileIndex(path string) int {
	for i, f := range m.files {
		if f.Path == path {
			return i
		}
	}
	return -1
}

// TestResumeOldComments checks the comments of a state file of v0.2.7,
// which has no anchors. A comment gets the revision of the saved scope, so
// that a comment in the commits scope still shows.
func TestResumeOldComments(t *testing.T) {
	m, _ := newTestModel(t)
	path := filepath.Join(t.TempDir(), "state.json")
	state := `{"tab":0,"scope":"commits","from":"aaa","to":"bbb",` +
		`"comments":{"last":1,"items":[{"id":1,"path":"a.go","side":1,` +
		`"start":2,"end":2,"text":"x","stale":false,"code":["b"],"snippet":null}]}}`
	if err := os.WriteFile(path, []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := loadState(path)
	if err != nil {
		t.Fatalf("loadState: %v", err)
	}
	m.restoreSettings(s)
	want := diff.Anchor{InDiff: true, Rev: "aaa..bbb"}
	if c := m.comments.Find(1); c == nil || c.Anchor != want {
		t.Errorf("comment after the restore = %+v, want anchor %+v", c, want)
	}
	if m.anchor() != want {
		t.Errorf("anchor of the view = %+v, want %+v", m.anchor(), want)
	}
}
