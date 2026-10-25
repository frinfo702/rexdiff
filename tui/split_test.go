package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// newSplitModel returns a test model that chooses the split layout, with
// a pane w columns wide.
func newSplitModel(t *testing.T, w int) model {
	t.Helper()
	m, _ := newTestModel(t)
	m.split = true
	return step(t, m, tea.WindowSizeMsg{Width: w, Height: 40})
}

// TestSplitNeedsWidth checks that a narrow pane shows one column, and that
// | tells why. The choice of split stays.
func TestSplitNeedsWidth(t *testing.T) {
	m := newSplitModel(t, 120)
	if m.splitShown() {
		t.Errorf("width 120 (code %d): split shows, want unified", m.codeWidth())
	}
	if m = press(t, m, "|"); m.status != "too narrow for split" || !m.split {
		t.Errorf("| at width 120: status %q, split %v; want \"too narrow for split\", true",
			m.status, m.split)
	}
}

// TestSplitFollowsWidth checks that split shows again when the pane gets
// wide again.
func TestSplitFollowsWidth(t *testing.T) {
	m := newSplitModel(t, 210)
	if !m.splitShown() {
		t.Fatalf("width 210 (code %d): unified shows, want split", m.codeWidth())
	}
	if m = step(t, m, tea.WindowSizeMsg{Width: 120, Height: 40}); m.splitShown() {
		t.Error("after a resize to 120: split shows, want unified")
	}
	if m = step(t, m, tea.WindowSizeMsg{Width: 210, Height: 40}); !m.splitShown() {
		t.Error("after a resize to 210: unified shows, want split")
	}
	if m = press(t, m, "|"); m.split || m.splitShown() {
		t.Error("| at width 210: split shows, want unified")
	}
	if m = press(t, m, "|"); !m.splitShown() {
		t.Error("| again at width 210: unified shows, want split")
	}
}

// TestSplitAddedFile checks that a file with lines on one side shows one
// column, and that | does nothing.
func TestSplitAddedFile(t *testing.T) {
	m := newSplitModel(t, 210)
	m = press(t, m, "b") // the branch scope adds b.go
	i := -1
	for j, f := range m.files {
		if f.Path == "b.go" {
			i = j
		}
	}
	if i < 0 {
		t.Fatalf("the branch scope has %v, want b.go", paths(m))
	}
	m.pickFile(i)
	if m.splitShown() {
		t.Error("an added file: split shows, want unified")
	}
	if m = press(t, m, "|"); m.status != "" || !m.split {
		t.Errorf("| on an added file: status %q, split %v; want no status, true",
			m.status, m.split)
	}
}
