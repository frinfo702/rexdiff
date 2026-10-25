package tui

import (
	"testing"
)

// TestCommitPicker checks the commit picker of herdr-reviewr: no filter,
// v anchors a range, and esc clears the anchor, then closes.
func TestCommitPicker(t *testing.T) {
	m, dir := newTestModel(t)
	for _, s := range []string{"c1", "c2", "c3"} {
		git(t, dir, "commit", "-q", "--allow-empty", "-m", s)
	}
	m = press(t, m, "G")
	if m.overlay == nil || m.overlay.kind != overlayCommits {
		t.Fatal("G: no commit picker, want the picker")
	}
	if !shows(m, "commits · 4 over main", "enter open · esc cancel · j k move · v select") {
		t.Errorf("the picker got:\n%s\nwant the title and the keys", view(m))
	}
	// The picker has no filter, so a letter does not hide rows.
	if m = press(t, m, "x"); len(m.overlay.shown) != 4 || m.overlay.input.Value() != "" {
		t.Errorf("x in the picker: %d rows, input %q; want 4 rows, no input",
			len(m.overlay.shown), m.overlay.input.Value())
	}

	m = press(t, m, "v", "j", "j")
	if m.overlay.mark != 0 || m.overlay.cur != 2 || !shows(m, "enter open 3 · esc clear") {
		t.Errorf("v j j: mark %d, cursor %d, view:\n%s\nwant mark 0, cursor 2",
			m.overlay.mark, m.overlay.cur, view(m))
	}
	if m = press(t, m, "esc"); m.overlay == nil || m.overlay.mark != -1 {
		t.Fatalf("esc with an anchor: overlay %v, want it open with no anchor", m.overlay)
	}
	if m = press(t, m, "esc"); m.overlay != nil {
		t.Fatal("esc with no anchor: the picker stays open, want it closed")
	}

	// A range of two commits, then the picker opens on it.
	m = press(t, m, "G", "j", "v", "k", "enter")
	items := func(m model) []pickItem { return m.overlay.items }
	m = press(t, m, "G")
	if m.scope != scopeCommits || m.to != items(m)[0].id || m.from != items(m)[1].id {
		t.Errorf("the pick: from %s to %s, want the two newest commits", m.from, m.to)
	}
	if m.overlay.cur != 0 || m.overlay.mark != 1 {
		t.Errorf("G on a pick: cursor %d, mark %d; want 0, 1", m.overlay.cur, m.overlay.mark)
	}
}

// TestCommitPickerOnBase checks that the picker lists the last commits on
// the base branch.
func TestCommitPickerOnBase(t *testing.T) {
	m, dir := newTestModel(t)
	git(t, dir, "stash", "-q")
	git(t, dir, "checkout", "-q", "main")
	if m = press(t, m, "G"); !shows(m, "commits · last 50") || len(m.overlay.shown) != 1 {
		t.Errorf("G on main: view:\n%s\nwant the last 50 commits", view(m))
	}
}
